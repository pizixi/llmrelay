package config

import (
	"path/filepath"
	"testing"
)

func TestSQLiteConfigRoundTripAndLegacyMigration(t *testing.T) {
	tempDir := t.TempDir()
	legacyPath := filepath.Join(tempDir, "config.json")
	databasePath := filepath.Join(tempDir, "llmrelay.db")
	want := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://example.com/v1", APIType: UpstreamOpenAI, CustomModels: []string{"gpt-test"}},
		},
		UpstreamOrder:   []string{"primary"},
		DefaultUpstream: "primary",
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{Upstream: "primary", TargetModel: "gpt-test", Weight: 1}}},
		},
	}
	if err := SaveConfig(legacyPath, want); err != nil {
		t.Fatalf("save legacy config: %v", err)
	}
	if err := MigrateLegacyJSON(databasePath, legacyPath); err != nil {
		t.Fatalf("migrate legacy config: %v", err)
	}
	got, err := LoadConfig(databasePath)
	if err != nil {
		t.Fatalf("load sqlite config: %v", err)
	}
	if got.DefaultUpstream != "primary" || got.Upstreams["primary"] == nil {
		t.Fatalf("unexpected migrated config: %#v", got)
	}
	if target := got.ModelAlias["chat"].Targets; len(target) != 1 || target[0].TargetModel != "gpt-test" {
		t.Fatalf("unexpected migrated alias: %#v", target)
	}

	got.Upstreams["secondary"] = &UpstreamConfig{BaseURL: "https://secondary.example/v1", APIType: UpstreamResponses}
	got.UpstreamOrder = []string{"primary", "secondary"}
	if err := SaveConfig(databasePath, got); err != nil {
		t.Fatalf("save sqlite config: %v", err)
	}
	reloaded, err := LoadConfig(databasePath)
	if err != nil {
		t.Fatalf("reload sqlite config: %v", err)
	}
	if reloaded.Upstreams["secondary"] == nil {
		t.Fatal("sqlite round trip lost secondary upstream")
	}
}
