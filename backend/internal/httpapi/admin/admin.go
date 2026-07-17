package admin

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	catalogpkg "llmrelay/backend/internal/catalog"
)

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

type adminModelTestRequest struct {
	Upstream string `json:"upstream"`
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
}

func writeAdminJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func modelTestRequestBody(apiType UpstreamType, model, prompt string) []byte {
	var payload map[string]any
	switch apiType {
	case UpstreamAnthropic:
		payload = map[string]any{
			"model":      model,
			"messages":   []map[string]any{{"role": "user", "content": prompt}},
			"max_tokens": 1024,
			"stream":     true,
		}
	case UpstreamResponses:
		payload = map[string]any{
			"model":             model,
			"input":             prompt,
			"max_output_tokens": 1024,
			"stream":            true,
		}
	default:
		payload = map[string]any{
			"model":      model,
			"messages":   []map[string]any{{"role": "user", "content": prompt}},
			"max_tokens": 1024,
			"stream":     true,
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

func proxyAdminModelTestStream(w http.ResponseWriter, body io.ReadCloser) {
	defer body.Close()
	setSSEHeaders(w.Header())
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	buffer := make([]byte, 16<<10)
	for {
		read, readErr := body.Read(buffer)
		if read > 0 {
			if _, writeErr := w.Write(buffer[:read]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				emitOpenAIStreamError(w, flusher, map[string]any{"message": readErr.Error()}, "failed to read upstream model test stream")
			}
			return
		}
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
	input.Model = strings.TrimSpace(input.Model)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Upstream == "" || input.Model == "" || input.Prompt == "" {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "upstream, model and prompt are required"})
		return
	}
	if len(input.Upstream) > 128 || len(input.Model) > 512 || utf8.RuneCountInString(input.Prompt) > 32768 {
		writeAdminJSON(w, http.StatusBadRequest, map[string]any{"error": "upstream, model or prompt is too long"})
		return
	}

	upstreams, _ := getConfiguredUpstreams()
	upstream := upstreams[input.Upstream]
	if upstream == nil {
		writeAdminJSON(w, http.StatusNotFound, map[string]any{"error": "upstream not found"})
		return
	}
	zeroRetries := 0
	upstream.MaxRetries = &zeroRetries

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	body, status, _, callErr := callPreparedUpstreamStream(
		ctx,
		modelTestRequestBody(upstream.APIType, input.Model, input.Prompt),
		input.Upstream,
		input.Model,
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
		})
		return
	}

	w.Header().Set("X-Model-Test-Protocol", string(upstream.APIType))
	w.Header().Set("X-Upstream-Status", strconv.Itoa(status))
	switch upstream.APIType {
	case UpstreamAnthropic:
		anthropicStreamToChatHandler(w, body, input.Model, input.Model, false)
	case UpstreamResponses:
		responsesStreamToChatHandler(w, body, input.Model, input.Model, false)
	default:
		proxyAdminModelTestStream(w, body)
	}
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
			"active_socks5":                cfg.ActiveSocks5,
			"upstreams":                    cfg.Upstreams,
			"upstream_order":               cfg.UpstreamOrder,
			"default_upstream":             cfg.DefaultUpstream,
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
		reconfigureCatalog(cfg.Upstreams)
		if debugMode {
			log.Printf("配置已更新：别名=%d，推理强度映射=%d，上游=%d，默认上游=%s", len(cfg.ModelAlias), len(cfg.ReasoningEffortMap), len(cfg.Upstreams), cfg.DefaultUpstream)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
