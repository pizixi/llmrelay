package config

import (
	"path/filepath"
	"reflect"
	"testing"

	"llmrelay/backend/internal/storage"
)

func TestModelRoutingPrioritizesWeightAndPreservesWeightedOption(t *testing.T) {
	previous := Snapshot()
	t.Cleanup(func() { ApplyConfig(previous) })
	cfg := AppConfig{
		Upstreams: map[string]*UpstreamConfig{"primary": {BaseURL: "https://primary.example"}, "backup": {BaseURL: "https://backup.example"}},
		ModelAlias: map[string]ModelAlias{"alias": {Targets: []ModelAliasTarget{
			{Upstream: "backup", TargetModel: "backup", Weight: 1, Enabled: true},
			{Upstream: "primary", TargetModel: "disabled", Weight: 100, Enabled: false},
			{Upstream: "primary", TargetModel: "first", Weight: 3, Enabled: true},
			{Upstream: "primary", TargetModel: "same-weight", Weight: 3, Enabled: true},
		}}},
	}
	ApplyConfig(cfg)
	for index := range 8 {
		routes, matched, policy := ResolveRequestModelCandidates("alias")
		first, second := "first", "same-weight"
		if index%2 == 1 {
			first, second = second, first
		}
		if !matched || policy.Strategy != RoutingPriority || policy.FailureThreshold != 3 || policy.CooldownSeconds != 180 || len(routes) != 3 || routes[0].Model != first || routes[1].Model != second || routes[2].Model != "backup" {
			t.Fatalf("priority candidates = %#v, policy = %#v", routes, policy)
		}
	}
	cfg.ModelRouting.Strategy = RoutingWeightedRoundRobin
	ApplyConfig(cfg)
	for index, expected := range []string{"backup", "first", "first", "first", "same-weight", "same-weight", "same-weight"} {
		model, _, _, _, _, matched := ResolveRequestModel("alias")
		if !matched || model != expected {
			t.Fatalf("weighted request %d = %q, want %q", index, model, expected)
		}
	}
}

func TestModelRoutingConfigSQLiteAndLegacyDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.db")
	want := ModelRoutingConfig{Strategy: RoutingWeightedRoundRobin, DisableFailover: true, MaxAttempts: 4, DisableCooldown: true, FailureThreshold: 3, CooldownSeconds: 120}
	if err := SaveConfig(path, AppConfig{ModelRouting: want}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(path)
	if err != nil || !reflect.DeepEqual(loaded.ModelRouting, want) {
		t.Fatalf("round trip policy=%#v err=%v", loaded.ModelRouting, err)
	}
	db, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE model_routing_settings"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	loaded, err = LoadConfig(path)
	if err != nil || loaded.ModelRouting != NormalizeModelRoutingConfig(ModelRoutingConfig{}) {
		t.Fatalf("legacy defaults=%#v err=%v", loaded.ModelRouting, err)
	}
}

func TestModelRoutingConfigRejectsInvalidValues(t *testing.T) {
	for _, value := range []ModelRoutingConfig{
		{Strategy: "random"}, {MaxAttempts: -1}, {MaxAttempts: 1001},
		{FailureThreshold: -1}, {FailureThreshold: 101}, {CooldownSeconds: -1}, {CooldownSeconds: 86401},
	} {
		if err := ValidateConfig(&AppConfig{ModelRouting: value}); err == nil {
			t.Fatalf("accepted invalid policy %#v", value)
		}
	}
}
