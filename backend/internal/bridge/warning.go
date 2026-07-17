package bridge

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"llmrelay/backend/internal/domain"
	"llmrelay/backend/internal/protocol/anthropic"
	"llmrelay/backend/internal/protocol/chat"
	"llmrelay/backend/internal/protocol/responses"
)

type OpenAIRequest = chat.Request
type Message = chat.Message
type ClaudeTool = anthropic.Tool
type ResponsesTool = responses.Tool
type UpstreamConfig = domain.UpstreamConfig
type BridgeMode = domain.BridgeMode

const (
	BridgeModeCompatible = domain.BridgeModeCompatible
	BridgeModeStrict     = domain.BridgeModeStrict
	UpstreamAnthropic    = domain.UpstreamAnthropic
)

func bridgeString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func effectiveUpstreamName(name string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return "default"
}

type BridgeWarning struct {
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	Message  string `json:"message"`
	Severity string `json:"severity,omitempty"`
}

func appendBridgeWarning(warnings []BridgeWarning, warning BridgeWarning) []BridgeWarning {
	if warning.Severity == "" {
		warning.Severity = bridgeWarningSeverity(warning.Code)
	}
	for _, existing := range warnings {
		if existing.Code == warning.Code && existing.Path == warning.Path && existing.Message == warning.Message {
			return warnings
		}
	}
	return append(warnings, warning)
}

func bridgeWarningSeverity(code string) string {
	switch code {
	case "custom_tool_emulated", "tool_name_rewritten", "chat_option_auto_downgraded",
		"output_hint_ignored", "prompt_cache_hint_ignored", "system_cache_control_ignored",
		"tool_cache_control_ignored", "hosted_web_search_fallback":
		return "info"
	default:
		return "degraded"
	}
}

func openAIServiceTierFromAnthropic(value any) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(bridgeString(value))) {
	case "auto":
		return "auto", true
	case "standard_only":
		return "default", true
	default:
		return "", false
	}
}

func anthropicServiceTierFromOpenAI(value any) (tier string, recognized, approximated bool) {
	switch strings.ToLower(strings.TrimSpace(bridgeString(value))) {
	case "auto":
		return "auto", true, false
	case "default":
		return "standard_only", true, false
	case "priority", "flex", "scale":
		return "auto", true, true
	default:
		return "", false, false
	}
}

type bridgeWarningLogEntry struct {
	LastLogged time.Time
	Suppressed int
}

var (
	bridgeWarningLogMu      sync.Mutex
	bridgeWarningLogEntries = map[string]bridgeWarningLogEntry{}
)

const (
	bridgeWarningLogWindow     = 5 * time.Minute
	bridgeWarningLogMaxEntries = 2048
)

// logBridgeWarnings 保证每个请求的警告可观测，同时避免智能体客户端每轮重复发送
// 相同工具时刷屏。每个请求仍会输出响应头和响应体中的警告元数据。
func logBridgeWarnings(scope, upstreamName, model string, warnings []BridgeWarning) {
	now := time.Now()
	for _, warning := range warnings {
		severity := warning.Severity
		if severity == "" {
			severity = bridgeWarningSeverity(warning.Code)
		}
		key := strings.Join([]string{scope, effectiveUpstreamName(upstreamName), model, warning.Code, warning.Path}, "\x00")
		bridgeWarningLogMu.Lock()
		if len(bridgeWarningLogEntries) >= bridgeWarningLogMaxEntries {
			for existingKey, existingEntry := range bridgeWarningLogEntries {
				if now.Sub(existingEntry.LastLogged) >= 2*bridgeWarningLogWindow {
					delete(bridgeWarningLogEntries, existingKey)
				}
			}
			// 动态模型名和路径名不能让警告限流器成为进程生命周期内无上限的缓存。
			// 丢弃抑制状态是安全的。
			for existingKey := range bridgeWarningLogEntries {
				if len(bridgeWarningLogEntries) < bridgeWarningLogMaxEntries {
					break
				}
				delete(bridgeWarningLogEntries, existingKey)
			}
		}
		entry := bridgeWarningLogEntries[key]
		if !entry.LastLogged.IsZero() && now.Sub(entry.LastLogged) < bridgeWarningLogWindow {
			entry.Suppressed++
			bridgeWarningLogEntries[key] = entry
			bridgeWarningLogMu.Unlock()
			continue
		}
		suppressed := entry.Suppressed
		bridgeWarningLogEntries[key] = bridgeWarningLogEntry{LastLogged: now}
		bridgeWarningLogMu.Unlock()

		if suppressed > 0 {
			log.Printf("[%s 协议桥接警告] 上游=%s 模型=%s 级别=%s 代码=%s 路径=%s 已抑制=%d 消息=%s", scope, effectiveUpstreamName(upstreamName), model, severity, warning.Code, warning.Path, suppressed, warning.Message)
		} else {
			log.Printf("[%s 协议桥接警告] 上游=%s 模型=%s 级别=%s 代码=%s 路径=%s 消息=%s", scope, effectiveUpstreamName(upstreamName), model, severity, warning.Code, warning.Path, warning.Message)
		}
	}
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func prependBridgeGuidance(messages []Message, guidance []string) []Message {
	if len(guidance) == 0 {
		return messages
	}
	content := "llm2api compatibility notice:\n- " + strings.Join(guidance, "\n- ")
	return append([]Message{{Role: "system", Content: content}}, messages...)
}

func anthropicServerToolGuidance(tools []ClaudeTool) []string {
	var guidance []string
	for _, tool := range tools {
		toolType := strings.TrimSpace(tool.Type)
		if toolType == "" {
			continue
		}
		if strings.Contains(strings.ToLower(toolType), "web_search") {
			guidance = appendUniqueString(guidance, "Anthropic hosted web search is unavailable on this upstream. Do not claim that web search was performed; continue from the available context and state the limitation when current external information is essential.")
			continue
		}
		guidance = appendUniqueString(guidance, fmt.Sprintf("Anthropic server tool %q is unavailable on this upstream. Do not claim that it was executed; use the remaining tools or explain the limitation.", toolType))
	}
	return guidance
}

func responsesHostedToolGuidance(tools []ResponsesTool) []string {
	var guidance []string
	for _, tool := range tools {
		toolType := strings.TrimSpace(tool.Type)
		switch toolType {
		case "", "function", "custom", "namespace", "tool_search":
			continue
		case "web_search", "web_search_preview":
			guidance = appendUniqueString(guidance, "Hosted web search is unavailable on this upstream. Do not claim that web search was performed; continue from the available context and state the limitation when current external information is essential.")
		default:
			guidance = appendUniqueString(guidance, fmt.Sprintf("Hosted tool %q is unavailable on this upstream. Do not claim that it was executed; use the remaining tools or explain the limitation.", toolType))
		}
	}
	return guidance
}

func appendBridgeWarnings(warnings []BridgeWarning, additions ...[]BridgeWarning) []BridgeWarning {
	for _, group := range additions {
		for _, warning := range group {
			warnings = appendBridgeWarning(warnings, warning)
		}
	}
	return warnings
}

func writeBridgeWarningHeaders(header http.Header, warnings []BridgeWarning) {
	header.Del("X-Llm2api-Warning")
	header.Del("X-Llm2api-Warning-Count")
	if len(warnings) == 0 {
		return
	}
	header.Set("X-Llm2api-Bridge-Mode", "compatible")
	header.Set("X-Llm2api-Warning-Count", strconv.Itoa(len(warnings)))
	for _, warning := range warnings {
		severity := warning.Severity
		if severity == "" {
			severity = bridgeWarningSeverity(warning.Code)
		}
		value := warning.Code + "; severity=" + severity
		if warning.Path != "" {
			value += "; path=" + warning.Path
		}
		value = strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
		header.Add("X-Llm2api-Warning", value)
	}
}

func applyBridgeWarnings(response map[string]any, warnings []BridgeWarning) {
	if response != nil && len(warnings) > 0 {
		response["llm2api_warnings"] = warnings
	}
}

func bridgeWarningsError(warnings []BridgeWarning) error {
	if len(warnings) == 0 {
		return nil
	}
	paths := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		path := warning.Path
		if path == "" {
			path = warning.Code
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return fmt.Errorf("selected upstream requires lossy bridge conversion for: %s", strings.Join(paths, ", "))
}

// effectiveBridgeMode 合并持久化上游策略与单次请求头。
// 客户端可以将 compatible 上游收紧为 strict，但不能放宽管理员配置的 strict 策略。
func effectiveBridgeMode(r *http.Request, upstream *UpstreamConfig) BridgeMode {
	if upstream != nil && strings.EqualFold(strings.TrimSpace(string(upstream.BridgeMode)), string(BridgeModeStrict)) {
		return BridgeModeStrict
	}
	if r != nil && strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Llm2api-Bridge-Mode")), string(BridgeModeStrict)) {
		return BridgeModeStrict
	}
	return BridgeModeCompatible
}

func rejectStrictBridgeWarnings(w http.ResponseWriter, r *http.Request, warnings []BridgeWarning) bool {
	if len(warnings) == 0 {
		return false
	}
	w.Header().Set("X-Llm2api-Bridge-Mode", string(BridgeModeStrict))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	message := bridgeWarningsError(warnings).Error()
	if r.URL.Path == "/v1/messages" {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type":  "error",
			"error": map[string]any{"type": "invalid_request_error", "message": message},
		})
	} else {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"type": "invalid_request_error", "message": message},
		})
	}
	return true
}

func chatBridgeWarnings(req *OpenAIRequest, upstream *UpstreamConfig) []BridgeWarning {
	if req == nil || upstream == nil || upstream.APIType != UpstreamAnthropic {
		return nil
	}
	var warnings []BridgeWarning
	if value, exists := req.AdditionalFields["service_tier"]; exists && value != nil {
		_, recognized, approximated := anthropicServiceTierFromOpenAI(value)
		switch {
		case !recognized:
			warnings = appendBridgeWarning(warnings, BridgeWarning{
				Code: "request_field_ignored", Path: "service_tier",
				Message: fmt.Sprintf("Chat service_tier %q has no Anthropic equivalent and was ignored", bridgeString(value)),
			})
		case approximated:
			warnings = appendBridgeWarning(warnings, BridgeWarning{
				Code: "service_tier_approximated", Path: "service_tier",
				Message: fmt.Sprintf("Chat service_tier %q was mapped to Anthropic auto", bridgeString(value)),
			})
		}
	}
	for index, rawTool := range req.RawTools {
		tool, _ := rawTool.(map[string]any)
		toolType, _ := tool["type"].(string)
		if toolType != "" && toolType != "function" {
			warnings = appendBridgeWarning(warnings, BridgeWarning{
				Code: "unsupported_chat_tool", Path: fmt.Sprintf("tools[%d]", index),
				Message: fmt.Sprintf("Chat tool type %q cannot be represented by Anthropic and was skipped", toolType),
			})
		}
	}
	return warnings
}
