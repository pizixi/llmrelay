// Package app 是 LLM Relay 后端的依赖组装和进程生命周期入口。
package app

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"llmrelay/backend/internal/auth"
	"llmrelay/backend/internal/catalog"
	"llmrelay/backend/internal/config"
	"llmrelay/backend/internal/gateway"
	"llmrelay/backend/internal/httpapi"
	"llmrelay/backend/internal/httpapi/admin"
	"llmrelay/backend/internal/stats"
)

const applicationName = "LLM Relay"

func defaultAPIAccessKey() string {
	if value := os.Getenv("LLMGATEWAYGO_API_KEY"); value != "" {
		return value
	}
	return os.Getenv("LLM2API_API_KEY")
}

// Run 配置并启动 LLM Relay HTTP 服务。
func Run(assets embed.FS) {
	port := "8000"
	databasePath := "llmrelay.db"
	adminPassword := "123456"
	apiAccessKey := defaultAPIAccessKey()
	debugMode := false
	debugLogBodies := false
	flag.StringVar(&port, "port", port, "服务端口")
	flag.StringVar(&databasePath, "db", databasePath, "SQLite 数据库路径")
	flag.StringVar(&adminPassword, "password", adminPassword, "管理面板密码（留空则不启用登录验证）")
	flag.StringVar(&apiAccessKey, "api-key", apiAccessKey, "对外 /v1 API 密钥（也可用 LLMGATEWAYGO_API_KEY；留空保持兼容）")
	flag.BoolVar(&debugMode, "debug", false, "启用调试日志")
	flag.BoolVar(&debugLogBodies, "debug-log-bodies", false, "在调试日志中记录请求和响应正文（可能包含敏感信息）")
	flag.Parse()
	config.SetPath(databasePath)
	cfg, err := config.LoadConfig(databasePath)
	if err != nil {
		log.Fatalf("无法加载配置 %s: %v（原文件未被修改）", databasePath, err)
	}
	legacyAPIKeyMigrated := config.MigrateLegacyAPIKey(&cfg, apiAccessKey)
	config.ApplyConfig(cfg)
	if legacyAPIKeyMigrated {
		log.Printf("旧版对外 API Key 已迁移到 API 密钥管理")
	}
	if err := config.SaveConfig(databasePath, cfg); err != nil {
		log.Printf("警告: 无法保存配置: %v", err)
	}

	gateway.SetDebug(debugMode, debugLogBodies)
	legacyAuthKey := apiAccessKey
	for _, managedKey := range cfg.APIKeys {
		if strings.TrimSpace(managedKey.Key) == strings.TrimSpace(apiAccessKey) {
			// Once the legacy value has been imported, deletion and disabling
			// must be controlled by the API key page rather than the old flag.
			legacyAuthKey = ""
			break
		}
	}
	auth.Configure(adminPassword, legacyAuthKey)
	auth.SetAPIKeys(cfg.APIKeys)
	admin.Configure(assets, debugMode)
	auth.SetLoginRenderer(admin.RenderLoginPage)
	stats.SetPath(databasePath)
	stats.LoadTokenStats()

	models := catalog.Initialize()
	log.Printf("配置已从 SQLite %s 加载", databasePath)
	log.Printf("已加载 %d 个配置模型", len(models))
	for _, model := range models {
		log.Printf("  - %s", model.ID)
	}
	snapshot := config.Snapshot()
	log.Printf("%s", applicationName)
	log.Printf("===================")
	log.Printf("端口:     %s", port)
	log.Printf("上游:     %d 个（默认: %s）", len(snapshot.Upstreams), snapshot.DefaultUpstream)
	log.Printf("模型：  %d 个模型已加载", len(catalog.GetModelIDs()))
	log.Printf("别名：  %d", len(snapshot.ModelAlias))

	if adminPassword != "" {
		log.Printf("管理面板: http://localhost:%s/ （密码认证已启用）", port)
		if adminPassword == "123456" {
			log.Printf("警告: 管理面板仍使用默认密码，请通过 -password 修改")
		}
	} else {
		log.Printf("管理面板: http://localhost:%s/ （无密码）", port)
	}
	apiAuthEnabled := strings.TrimSpace(legacyAuthKey) != ""
	for _, managedKey := range cfg.APIKeys {
		if !managedKey.Disabled && strings.TrimSpace(managedKey.Key) != "" {
			apiAuthEnabled = true
			break
		}
	}
	if !apiAuthEnabled {
		log.Printf("警告: /v1 API 鉴权未启用；公网部署请设置 -api-key 或 LLMGATEWAYGO_API_KEY")
	} else {
		log.Printf("/v1 API: API 密钥鉴权已启用（%d 个密钥）", len(cfg.APIKeys))
	}
	log.Printf("===================")

	addr := ":" + port
	log.Printf("服务器启动在 %s", addr)
	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
