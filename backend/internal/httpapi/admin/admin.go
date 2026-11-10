package admin

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"llmrelay/backend/internal/auth"
	"llmrelay/backend/internal/bridge"
	catalogpkg "llmrelay/backend/internal/catalog"
	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/stats"
)

// BridgeCapabilitiesHandler reports the effective protocol capability matrix
// and each configured provider declaration. It is read-only so operators can
// inspect why a request was converted without changing routing state.
func BridgeCapabilitiesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	cfg := currentConfig()
	upstreams := map[string]any{}
	for name, upstream := range cfg.Upstreams {
		upstreams[name] = bridge.ProviderCapabilities(upstream)
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{
		"matrix":    bridge.CapabilityOutcomesMatrix(),
		"upstreams": upstreams,
	})
}

// ======================== Admin 管理页面 ========================

func ReloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	targetUpstream := strings.TrimSpace(r.URL.Query().Get("upstream"))
	if targetUpstream != "" {
		fetched, err := fetchModelsForUpstream(targetUpstream, true)
		if err != nil {
			log.Printf("刷新上游 %s 模型列表失败: %v", targetUpstream, err)
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		catalog, catalogErr := fetchModelsForUpstream(targetUpstream, false)
		if catalogErr != nil {
			log.Printf("刷新上游 %s 模型目录失败: %v", targetUpstream, catalogErr)
			http.Error(w, catalogErr.Error(), http.StatusBadGateway)
			return
		}
		effectiveCount, catalogCount, totalEffectiveCount, totalCatalogCount := catalogpkg.ApplyUpstreamRefresh(targetUpstream, fetched, catalog)
		log.Printf("上游 %s 模型列表已刷新: 有效 %d 个，目录 %d 个", targetUpstream, effectiveCount, catalogCount)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":                "ok",
			"upstream":              targetUpstream,
			"models":                effectiveCount,
			"upstream_models":       catalogCount,
			"total_models":          totalEffectiveCount,
			"total_upstream_models": totalCatalogCount,
			"upstreams":             getConfiguredUpstreamCount(),
		})
		return
	}

	fetched, err := fetchModels()
	if err == nil && len(fetched) > 0 {
		catalogpkg.ApplyEffective(fetched)
		log.Printf("模型列表已刷新: %d 个模型", len(fetched))
	} else if err != nil {
		log.Printf("刷新模型列表失败: %v", err)
	}
	catalog, catalogErr := fetchUpstreamModelCatalog()
	if catalogErr == nil {
		catalogpkg.ApplyCatalog(catalog)
		log.Printf("上游模型目录已刷新: %d 个模型", len(catalog))
	} else {
		log.Printf("刷新上游模型目录失败: %v", catalogErr)
	}
	effectiveCount, catalogCount := catalogpkg.Counts()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"models":          effectiveCount,
		"upstream_models": catalogCount,
		"upstreams":       getConfiguredUpstreamCount(),
	})
}

type adminUpstreamModelSyncResult struct {
	Upstream string   `json:"upstream"`
	Models   []string `json:"models"`
	Error    string   `json:"error,omitempty"`
}

func configuredUpstreamNames(cfg AppConfig) []string {
	names := make([]string, 0, len(cfg.Upstreams))
	seen := make(map[string]struct{}, len(cfg.Upstreams))
	for _, rawName := range cfg.UpstreamOrder {
		name := strings.TrimSpace(rawName)
		if name == "" || cfg.Upstreams[name] == nil {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	missing := make([]string, 0, len(cfg.Upstreams)-len(names))
	for name, upstream := range cfg.Upstreams {
		if upstream == nil {
			continue
		}
		if _, exists := seen[name]; !exists {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return append(names, missing...)
}

func uniqueSortedModelIDs(models []ModelInfo) []string {
	seen := make(map[string]struct{}, len(models))
	ids := make([]string, 0, len(models))
	for _, model := range models {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// AdminSyncModelsHandler fetches every upstream's real model catalog and
// returns independent results. It deliberately does not modify custom_models;
// the admin page presents the differences for confirmation before saving.
func AdminSyncModelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cfg := currentConfig()
	names := configuredUpstreamNames(cfg)
	results := make([]adminUpstreamModelSyncResult, len(names))
	var wait sync.WaitGroup
	for index, name := range names {
		wait.Add(1)
		go func(index int, name string) {
			defer wait.Done()
			models, err := fetchModelsForUpstream(name, false)
			result := adminUpstreamModelSyncResult{Upstream: name, Models: []string{}}
			if err != nil {
				result.Error = err.Error()
				results[index] = result
				return
			}
			result.Models = uniqueSortedModelIDs(models)
			applyUpstreamCatalogRefresh(name, models)
			results[index] = result
		}(index, name)
	}
	wait.Wait()

	succeeded := 0
	failed := 0
	for _, result := range results {
		if result.Error == "" {
			succeeded++
		} else {
			failed++
		}
	}
	status := "ok"
	if failed > 0 {
		status = "partial"
	}
	writeAdminJSON(w, http.StatusOK, map[string]any{
		"status":    status,
		"succeeded": succeeded,
		"failed":    failed,
		"upstreams": results,
	})
}

type adminModelTestRequest struct {
	Upstream string `json:"upstream"`
	// Alias tests a configured model mapping instead of a single upstream model.
	// The target upstream and model are resolved with the same weighted routing
	// used by real gateway requests.
	Alias    string                  `json:"alias"`
	Model    string                  `json:"model"`
	Prompt   string                  `json:"prompt"`
	Messages []adminModelTestMessage `json:"messages"`
}

type adminModelTestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func validModelTestMessages(messages []adminModelTestMessage) bool {
	if len(messages) == 0 || len(messages) > 41 || len(messages)%2 == 0 {
		return false
	}
	characters := 0
	for index, message := range messages {
		role := "user"
		if index%2 == 1 {
			role = "assistant"
		}
		if message.Role != role || strings.TrimSpace(message.Content) == "" {
			return false
		}
		characters += utf8.RuneCountInString(message.Content)
		if characters > 32768 {
			return false
		}
	}
	return true
}

func writeAdminJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// modelTestAnthropicMaxTokens 只用于 Anthropic 协议：该协议强制要求 max_tokens。
// 旧的 1024 上限会让推理型模型把额度全部消耗在思考阶段，最终只返回思考内容、
// 没有正文，管理页就会误报“模型未返回文本”。其余两种协议不限制输出长度，
// 由上游按模型默认值决定，避免再次截断。
const modelTestAnthropicMaxTokens = 4096

func modelTestRequestBody(apiType UpstreamType, model, prompt string, messages []adminModelTestMessage) []byte {
	conversation := messages
	if len(conversation) == 0 {
		conversation = []adminModelTestMessage{{Role: "user", Content: prompt}}
	}
	var payload map[string]any
	switch apiType {
	case UpstreamAnthropic:
		payload = map[string]any{
			"model":      model,
			"messages":   conversation,
			"max_tokens": modelTestAnthropicMaxTokens,
			"stream":     true,
		}
	case UpstreamResponses:
		var input any = prompt
		if len(messages) > 0 {
			input = conversation
		}
		payload = map[string]any{
			"model":  model,
			"input":  input,
			"stream": true,
		}
	default:
		payload = map[string]any{
			"model":          model,
			"messages":       conversation,
			"stream":         true,
			"stream_options": map[string]any{"include_usage": true},
		}
	}
	body, _ := json.Marshal(payload)
	return body
}

func modelTestErrorMessage(body []byte, callErr error) string {
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		if errObject, ok := payload["error"].(map[string]any); ok {
			if message, ok := errObject["message"].(string); ok && strings.TrimSpace(message) != "" {
				return truncatePreview(message, 600)
			}
		}
		if message, ok := payload["error"].(string); ok && strings.TrimSpace(message) != "" {
			return truncatePreview(message, 600)
		}
		if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
			return truncatePreview(message, 600)
		}
	}
	if callErr != nil {
		return truncatePreview(callErr.Error(), 600)
	}
	if message := strings.TrimSpace(string(body)); message != "" {
		return truncatePreview(message, 600)
	}
	return "upstream model test failed"
}

func proxyAdminModelTestStream(w http.ResponseWriter, body io.ReadCloser, model string) {
	setSSEHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	if err := proxyChatPassthroughStream(w, body, model, false); err != nil {
		flusher, _ := w.(http.Flusher)
		emitOpenAIStreamError(w, flusher, map[string]any{"message": err.Error()}, "failed to read upstream model test stream")
	}
}

func AdminTestModelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input adminModelTestRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 160<<10))
	if err := decoder.Decode(&input); err != nil {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	input.Upstream = strings.TrimSpace(input.Upstream)
	input.Alias = strings.TrimSpace(input.Alias)
	input.Model = strings.TrimSpace(input.Model)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if (input.Alias == "" && (input.Upstream == "" || input.Model == "")) ||
		(input.Prompt == "" && len(input.Messages) == 0) {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "alias or upstream and model, plus prompt or messages, are required"})
		return
	}
	if input.Messages != nil && !validModelTestMessages(input.Messages) {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "messages must alternate user and assistant, end with user, and contain at most 41 messages and 32768 characters"})
		return
	}
	if len(input.Upstream) > 128 || len(input.Alias) > 512 || len(input.Model) > 512 || utf8.RuneCountInString(input.Prompt) > 32768 {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "alias, upstream, model or prompt is too long"})
		return
	}

	upstreamName := input.Upstream
	model := input.Model
	var upstream *UpstreamConfig
	if input.Alias != "" {
		resolvedModel, _, resolvedUpstreamName, resolvedUpstream, aliasMatched, matched := config.ResolveRequestModel(input.Alias)
		if !aliasMatched {
			writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "model alias not found: " + input.Alias})
			return
		}
		if !matched || resolvedUpstream == nil || resolvedModel == "" {
			writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "model alias has no active target: " + input.Alias})
			return
		}
		upstreamName = resolvedUpstreamName
		model = resolvedModel
		upstream = resolvedUpstream
	} else {
		upstreams, _ := getConfiguredUpstreams()
		upstream = upstreams[input.Upstream]
		if upstream == nil {
			writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "upstream not found"})
			return
		}
	}
	zeroRetries := 0
	upstream.MaxRetries = &zeroRetries
	if input.Alias != "" {
		// 映射可能按权重指向不同上游；用响应头回传本次实际路由，便于
		// 管理页展示（失败时也能定位是哪个目标出错）。名称可能包含
		// 非 ASCII 字符，先转义再写入 Header。
		w.Header().Set("X-Model-Test-Upstream", url.PathEscape(upstreamName))
		w.Header().Set("X-Model-Test-Model", url.PathEscape(model))
	}

	// 测试请求不再限制输出额度后，推理型模型可能持续生成较长时间；
	// 客户端断开时 r.Context() 会立即取消，因此这里放宽总超时即可。
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	body, status, _, callErr := callPreparedUpstreamStream(
		ctx,
		modelTestRequestBody(upstream.APIType, model, input.Prompt, input.Messages),
		upstreamName,
		model,
		upstream,
		true,
	)
	if callErr != nil || status < 200 || status >= 300 {
		var errorBody []byte
		if body != nil {
			errorBody, _ = io.ReadAll(body)
			body.Close()
		}
		writeAdminJSON(w, http.StatusBadGateway, map[string]any{
			"error":           modelTestErrorMessage(errorBody, callErr),
			"upstream_status": status,
			"api_type":        upstream.APIType,
			"alias":           input.Alias,
			"upstream":        upstreamName,
			"model":           model,
		})
		return
	}

	w.Header().Set("X-Model-Test-Protocol", string(upstream.APIType))
	w.Header().Set("X-Upstream-Status", strconv.Itoa(status))
	// 管理页模型测试只用于连通性/效果验证，不属于真实业务调用，
	// 因此显式关闭用量统计，避免写入“使用记录”。
	proxyAdminModelTestResponse(w, body, upstream.APIType, model)
}

func AdminConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := currentConfig()
		w.Header().Set("Content-Type", "application/json")
		// 附带上游模型列表供管理面板下拉框使用
		availableModels := getAdminAvailableModels()
		if availableModels == nil {
			availableModels = []ModelInfo{}
		}
		availableModelsByUpstream := groupModelsByUpstream(availableModels)
		resp := map[string]any{
			"model_alias":                  cfg.ModelAlias,
			"reasoning_effort_map":         cfg.ReasoningEffortMap,
			"web_search":                   cfg.WebSearch,
			"socks5_proxies":               cfg.Socks5Proxies,
			"upstreams":                    cfg.Upstreams,
			"upstream_order":               cfg.UpstreamOrder,
			"available_models":             availableModels,
			"available_models_by_upstream": availableModelsByUpstream,
		}
		json.NewEncoder(w).Encode(resp)
	case http.MethodPost:
		configUpdateMu.Lock()
		defer configUpdateMu.Unlock()
		var cfg AppConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
			return
		}
		previous := currentConfig()
		if cfg.APIKeys == nil {
			// The general configuration form predates managed API keys and does
			// not submit them. Keep keys owned by /api/api-keys intact.
			cfg.APIKeys = previous.APIKeys
		}
		cleanup := reconcileRemovedUpstreamModels(previous, &cfg)
		if err := validateConfig(&cfg); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
			return
		}
		normalizeConfig(&cfg)
		if err := saveConfig(configPath(), cfg); err != nil {
			http.Error(w, `{"error":"Failed to save config"}`, http.StatusInternalServerError)
			return
		}
		applyConfig(cfg)
		auth.SetAPIKeys(cfg.APIKeys)
		reconfigureCatalog(cfg.Upstreams)
		if debugMode {
			log.Printf("配置已更新：别名=%d，推理强度映射=%d，上游=%d", len(cfg.ModelAlias), len(cfg.ReasoningEffortMap), len(cfg.Upstreams))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":                "ok",
			"model_mapping_cleanup": cleanup,
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

type adminAPIKeyRequest struct {
	Name     string `json:"name"`
	Disabled *bool  `json:"disabled"`
}

func apiKeyIDFromPath(r *http.Request) string {
	if r.URL.Path == "/api/api-keys" || r.URL.Path == "/api/keys" {
		return ""
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/api-keys/")
	if path == r.URL.Path {
		path = strings.TrimPrefix(r.URL.Path, "/api/keys/")
	}
	return strings.Trim(path, "/ ")
}

func saveAPIKeysConfig(cfg AppConfig) error {
	if err := validateConfig(&cfg); err != nil {
		return err
	}
	normalizeConfig(&cfg)
	if err := saveConfig(configPath(), cfg); err != nil {
		return err
	}
	applyConfig(cfg)
	auth.SetAPIKeys(cfg.APIKeys)
	return nil
}

func AdminAPIKeysHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := currentConfig()
		keys := cfg.APIKeys
		if keys == nil {
			keys = []APIKey{}
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"keys": keys})
		return
	case http.MethodPost:
		configUpdateMu.Lock()
		defer configUpdateMu.Unlock()
		var input adminAPIKeyRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		if err := decoder.Decode(&input); err != nil {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		name := strings.TrimSpace(input.Name)
		if name == "" || utf8.RuneCountInString(name) > 128 {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "密钥名称不能为空且不能超过 128 个字符"})
			return
		}
		cfg := currentConfig()
		for _, value := range cfg.APIKeys {
			if strings.EqualFold(strings.TrimSpace(value.Name), name) {
				writeAdminJSON(w, http.StatusConflict, map[string]any{"error": "密钥名称已存在"})
				return
			}
		}
		key, err := auth.GenerateAPIKey()
		if err != nil {
			writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": "生成 API 密钥失败"})
			return
		}
		id, err := auth.GenerateAPIKeyID()
		if err != nil {
			writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": "生成 API 密钥 ID 失败"})
			return
		}
		value := APIKey{ID: id, Name: name, Key: key}
		if input.Disabled != nil {
			value.Disabled = *input.Disabled
		}
		cfg.APIKeys = append(cfg.APIKeys, value)
		if err := saveAPIKeysConfig(cfg); err != nil {
			writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeAdminJSON(w, http.StatusCreated, map[string]any{"key": value})
		return
	case http.MethodPatch, http.MethodPut:
		configUpdateMu.Lock()
		defer configUpdateMu.Unlock()
		id := apiKeyIDFromPath(r)
		if id == "" {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "api key id is required"})
			return
		}
		var input adminAPIKeyRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
		if err := decoder.Decode(&input); err != nil {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		cfg := currentConfig()
		index := -1
		for candidate, value := range cfg.APIKeys {
			if value.ID == id {
				index = candidate
				break
			}
		}
		if index < 0 {
			writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "api key not found"})
			return
		}
		value := cfg.APIKeys[index]
		if input.Name != "" {
			name := strings.TrimSpace(input.Name)
			if name == "" || utf8.RuneCountInString(name) > 128 {
				writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "密钥名称不能为空且不能超过 128 个字符"})
				return
			}
			for candidate, other := range cfg.APIKeys {
				if candidate != index && strings.EqualFold(strings.TrimSpace(other.Name), name) {
					writeAdminJSON(w, http.StatusConflict, map[string]any{"error": "密钥名称已存在"})
					return
				}
			}
			value.Name = name
		}
		if input.Disabled != nil {
			value.Disabled = *input.Disabled
		}
		cfg.APIKeys[index] = value
		if err := saveAPIKeysConfig(cfg); err != nil {
			writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"key": value})
		return
	case http.MethodDelete:
		configUpdateMu.Lock()
		defer configUpdateMu.Unlock()
		id := apiKeyIDFromPath(r)
		if id == "" {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "api key id is required"})
			return
		}
		cfg := currentConfig()
		keys := make([]APIKey, 0, len(cfg.APIKeys))
		var removed APIKey
		found := false
		for _, value := range cfg.APIKeys {
			if value.ID == id {
				removed = value
				found = true
				continue
			}
			keys = append(keys, value)
		}
		if !found {
			writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "api key not found"})
			return
		}
		cfg.APIKeys = keys
		if err := saveAPIKeysConfig(cfg); err != nil {
			writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"status": "ok", "id": removed.ID})
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func AdminStatsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := json.Marshal(statsSnapshot())
		if err != nil {
			http.Error(w, `{"error":"marshal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	case http.MethodDelete:
		resetStats()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func AdminUsageHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		page, err := stats.ListUsageRecords(stats.UsageQuery{
			Limit:      limit,
			Offset:     offset,
			Model:      r.URL.Query().Get("model"),
			Upstream:   r.URL.Query().Get("upstream"),
			APIKeyName: firstQueryValue(r, "key_name", "api_key_name"),
			Date:       r.URL.Query().Get("date"),
		})
		if err != nil {
			writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeAdminJSON(w, http.StatusOK, page)
	case http.MethodDelete:
		id, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("id")), 10, 64)
		if err != nil || id <= 0 {
			writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "usage record id is required"})
			return
		}
		deleted, err := stats.DeleteUsageRecord(id)
		if err != nil {
			writeAdminJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		if !deleted {
			writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "usage record not found"})
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"status": "ok", "id": id})
	default:
		writeAdminJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

func firstQueryValue(r *http.Request, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(r.URL.Query().Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func FrontendAssetsHandler() http.Handler {
	assets, err := fs.Sub(frontendFS, "frontend/assets")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(assets)))
}

func writeFrontendPage(w http.ResponseWriter, name string) bool {
	page, err := frontendFS.ReadFile("frontend/pages/" + name)
	if err != nil {
		log.Printf("无法读取前端页面 %s: %v", name, err)
		http.Error(w, "frontend page not found", http.StatusInternalServerError)
		return false
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
	return true
}

func AdminPageHandler(w http.ResponseWriter, r *http.Request) {
	writeFrontendPage(w, "admin.html")
}

func RenderLoginPage(w http.ResponseWriter, msg string) {
	if !writeFrontendPage(w, "login.html") {
		return
	}
	if msg != "" {
		w.Write([]byte("<script>document.addEventListener('DOMContentLoaded',function(){var m=document.getElementById('login-msg');if(m){m.textContent='" + msg + "';m.style.display='block'}})</script>"))
	}
}
