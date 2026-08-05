package config

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"llmrelay/backend/internal/storage"
)

func TestSQLiteConfigRoundTripUsesNormalizedTables(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "llmrelay.db")
	maxRetries := 2
	want := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {
				BaseURL:                  "https://example.com/v1",
				APIKey:                   "key-a\nkey-b",
				APIType:                  UpstreamOpenAI,
				BridgeMode:               BridgeModeStrict,
				CustomModels:             []string{"gpt-test", "gpt-mini"},
				ResponsesReasoningFormat: "summary",
				MaxRetries:               &maxRetries,
			},
			"backup": {
				BaseURL:      "https://backup.example/v1",
				APIType:      UpstreamResponses,
				CustomModels: []string{"backup-model"},
			},
		},
		UpstreamOrder:   []string{"backup", "primary"},
		DefaultUpstream: "primary",
		ModelAlias: map[string]ModelAlias{
			"chat": {
				WithReasoning: true,
				Targets: []ModelAliasTarget{
					{Upstream: "primary", TargetModel: "gpt-test", Weight: 2},
					{Upstream: "backup", TargetModel: "backup-model", Weight: 1},
				},
				ReasoningEffortMap: map[string]string{"high": "max"},
			},
		},
		ReasoningEffortMap: map[string]string{"medium": "high"},
		WebSearch: WebSearchConfig{
			Enabled:             true,
			Provider:            "tavily",
			FallbackProvider:    "none",
			BaseURL:             "https://api.tavily.com/search",
			APIKey:              "search-key",
			SearXNGMode:         "auto",
			SearXNGDirectoryURL: "https://directory.example/instances.json",
			MaxResults:          5,
			TimeoutSeconds:      12,
			MaxToolRounds:       2,
			MaxResultBytes:      8192,
		},
		APIKeys:       []APIKey{{ID: "client-1", Name: "生产", Key: "client-secret", CreatedAt: "2026-08-05T00:00:00Z"}},
		Socks5Proxies: []Socks5Proxy{{Addr: "127.0.0.1:1080", Username: "user", Password: "pass", Name: "本地"}},
		ActiveSocks5:  "127.0.0.1:1080",
	}

	if err := SaveConfig(databasePath, want); err != nil {
		t.Fatalf("save sqlite config: %v", err)
	}
	got, err := LoadConfig(databasePath)
	if err != nil {
		t.Fatalf("load sqlite config: %v", err)
	}
	if got.DefaultUpstream != "primary" || got.Upstreams["primary"] == nil {
		t.Fatalf("unexpected config: %#v", got)
	}
	if got.UpstreamOrder[0] != "backup" || got.UpstreamOrder[1] != "primary" {
		t.Fatalf("upstream order = %#v", got.UpstreamOrder)
	}
	if got.Upstreams["primary"].APIKey != "key-a\nkey-b" || *got.Upstreams["primary"].MaxRetries != 2 {
		t.Fatalf("upstream credentials/retries were not preserved: %#v", got.Upstreams["primary"])
	}
	if target := got.ModelAlias["chat"].Targets; len(target) != 2 || target[0].Weight != 2 || target[1].Upstream != "backup" {
		t.Fatalf("unexpected alias targets: %#v", target)
	}
	if got.ActiveSocks5 != "127.0.0.1:1080" || got.APIKeys[0].Key != "client-secret" || !got.WebSearch.Enabled {
		t.Fatalf("non-upstream configuration was not preserved: %#v", got)
	}

	db, err := storage.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tableCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'app_config'").Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 {
		t.Fatal("normalized configuration must not retain the JSON app_config table")
	}
	for _, table := range []string{"upstreams", "upstream_api_keys", "upstream_models", "model_aliases", "model_alias_targets", "global_reasoning_effort_mappings", "alias_reasoning_effort_mappings", "api_keys", "socks5_proxies", "web_search_settings", "gateway_settings"} {
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&tableCount); err != nil {
			t.Fatal(err)
		}
		if tableCount != 1 {
			t.Fatalf("normalized table %q is missing", table)
		}
	}
}

func TestSQLiteConfigRejectsJSONFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveConfig(path, AppConfig{}); err == nil || !strings.Contains(err.Error(), "JSON config files") {
		t.Fatalf("SaveConfig accepted JSON path or returned wrong error: %v", err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "JSON config files") {
		t.Fatalf("LoadConfig accepted JSON path or returned wrong error: %v", err)
	}
}

func TestSQLiteConfigMigratesOnlyExistingJSONBlobTable(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "llmrelay.db")
	db, err := storage.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	legacy := AppConfig{
		Upstreams:       map[string]*UpstreamConfig{"primary": {BaseURL: "https://example.com/v1", APIType: UpstreamOpenAI}},
		UpstreamOrder:   []string{"primary"},
		DefaultUpstream: "primary",
		ModelAlias:      map[string]ModelAlias{"chat": {Targets: []ModelAliasTarget{{Upstream: "primary", TargetModel: "gpt-test", Weight: 1}}}},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE app_config (id INTEGER PRIMARY KEY CHECK (id = 1), data TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_config(id, data) VALUES (1, ?)`, string(data)); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConfig(databasePath)
	if err != nil {
		t.Fatalf("migrate config: %v", err)
	}
	if got.Upstreams["primary"] == nil || len(got.ModelAlias["chat"].Targets) != 1 {
		t.Fatalf("legacy blob was not migrated: %#v", got)
	}
	db, err = storage.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'app_config'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("legacy JSON blob table was not removed after migration")
	}
}

func TestSQLiteConfigEnforcesForeignKeyRelationships(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "llmrelay.db")
	db, err := storage.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatal("configuration database must enable foreign keys")
	}
	if err := SaveConfig(databasePath, AppConfig{Upstreams: map[string]*UpstreamConfig{"primary": {BaseURL: "https://example.com"}}}); err != nil {
		t.Fatal(err)
	}
	var upstreamCount, modelCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM upstreams").Scan(&upstreamCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM upstream_models").Scan(&modelCount); err != nil {
		t.Fatal(err)
	}
	if upstreamCount != 1 || modelCount != 0 {
		t.Fatalf("unexpected normalized counts: upstreams=%d models=%d", upstreamCount, modelCount)
	}
}

func TestSQLiteConfigPreservesUpstreamIDAcrossRename(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "llmrelay.db")
	initial := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {ID: 73, BaseURL: "https://example.com/v1", APIType: UpstreamOpenAI},
		},
		UpstreamOrder:   []string{"primary"},
		DefaultUpstream: "primary",
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{Upstream: "primary", TargetModel: "gpt-test", Weight: 1}}},
		},
	}
	if err := SaveConfig(databasePath, initial); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	renamed := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"main": {ID: 73, BaseURL: "https://example.com/v1", APIType: UpstreamOpenAI},
		},
		UpstreamOrder:   []string{"main"},
		DefaultUpstream: "main",
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{Upstream: "main", TargetModel: "gpt-test", Weight: 1}}},
		},
	}
	if err := SaveConfig(databasePath, renamed); err != nil {
		t.Fatalf("save renamed config: %v", err)
	}

	got, err := LoadConfig(databasePath)
	if err != nil {
		t.Fatalf("load renamed config: %v", err)
	}
	if got.Upstreams["main"] == nil || got.Upstreams["main"].ID != 73 {
		t.Fatalf("upstream id was not preserved: %#v", got.Upstreams)
	}
	if targets := got.ModelAlias["chat"].Targets; len(targets) != 1 || targets[0].Upstream != "main" {
		t.Fatalf("renamed alias target was not persisted: %#v", targets)
	}
}
