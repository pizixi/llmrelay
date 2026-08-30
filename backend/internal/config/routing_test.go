package config

import (
	"path/filepath"
	"testing"
)

func TestResolveRequestModelBalancesAcrossAllMatchingUpstreams(t *testing.T) {
	previous := Snapshot()
	t.Cleanup(func() { ApplyConfig(previous) })

	ApplyConfig(AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", CustomModels: []string{"shared-model"}},
			"backup":  {BaseURL: "https://backup.example/v1", CustomModels: []string{"shared-model"}},
			"other":   {BaseURL: "https://other.example/v1", CustomModels: []string{"other-model"}},
		},
		UpstreamOrder: []string{"primary", "backup", "other"},
	})

	for index, want := range []string{"primary", "backup", "primary", "backup"} {
		model, alias, upstreamName, upstream, aliasMatched, matched := ResolveRequestModel("shared-model")
		if model != "shared-model" || len(alias.Targets) != 0 || alias.TargetModel != "" || alias.Upstream != "" || aliasMatched || !matched || upstreamName != want || upstream == nil {
			t.Fatalf("request %d route = model=%q alias=%#v upstream=%q config=%#v aliasMatched=%t matched=%t; want %q", index+1, model, alias, upstreamName, upstream, aliasMatched, matched, want)
		}
	}

	if _, _, upstreamName, _, _, matched := ResolveRequestModel("missing-model"); matched || upstreamName != "primary" {
		t.Fatalf("unknown model route = upstream=%q matched=%t; want first upstream for error context and no match", upstreamName, matched)
	}
}

func TestResolveRequestModelMappingWinsOverAutomaticModelBalance(t *testing.T) {
	previous := Snapshot()
	t.Cleanup(func() { ApplyConfig(previous) })

	ApplyConfig(AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", CustomModels: []string{"request-model", "target-model"}},
			"backup":  {BaseURL: "https://backup.example/v1", CustomModels: []string{"request-model", "target-model"}},
		},
		UpstreamOrder: []string{"primary", "backup"},
		ModelAlias: map[string]ModelAlias{
			"request-model": {Targets: []ModelAliasTarget{{Upstream: "backup", TargetModel: "target-model", Weight: 1}}},
		},
	})

	model, alias, upstreamName, _, aliasMatched, matched := ResolveRequestModel("request-model")
	if model != "target-model" || alias.TargetModel != "target-model" || upstreamName != "backup" || !aliasMatched || !matched {
		t.Fatalf("mapped route = model=%q alias=%#v upstream=%q aliasMatched=%t matched=%t", model, alias, upstreamName, aliasMatched, matched)
	}
}

func TestResolveRequestModelDoesNotFallbackForInactiveAlias(t *testing.T) {
	previous := Snapshot()
	t.Cleanup(func() { ApplyConfig(previous) })

	ApplyConfig(AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", CustomModels: []string{"request-model", "target-a"}},
			"backup":  {BaseURL: "https://backup.example/v1", CustomModels: []string{"request-model", "target-b"}},
		},
		UpstreamOrder: []string{"primary", "backup"},
		ModelAlias: map[string]ModelAlias{
			"request-model": {Targets: []ModelAliasTarget{
				{Upstream: "primary", TargetModel: "target-a", Weight: 0},
				{Upstream: "backup", TargetModel: "target-b", Weight: 0},
			}},
		},
	})

	model, alias, upstreamName, upstream, aliasMatched, matched := ResolveRequestModel("request-model")
	if model != "request-model" || !aliasMatched || matched || upstreamName != "" || upstream != nil {
		t.Fatalf("inactive alias route = model=%q alias=%#v upstream=%q config=%#v aliasMatched=%t matched=%t", model, alias, upstreamName, upstream, aliasMatched, matched)
	}
}

func TestUpstreamProxyRoundTripsThroughSQLite(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "llmrelay.db")
	want := AppConfig{
		Socks5Proxies: []Socks5Proxy{{Addr: "127.0.0.1:1080", Name: "egress-a"}},
		Upstreams: map[string]*UpstreamConfig{
			"primary": {
				BaseURL:      "https://primary.example/v1",
				Proxy:        "127.0.0.1:1080",
				CustomModels: []string{"shared-model"},
			},
		},
		UpstreamOrder: []string{"primary"},
	}

	if err := SaveConfig(databasePath, want); err != nil {
		t.Fatalf("save config: %v", err)
	}
	got, err := LoadConfig(databasePath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got.Upstreams["primary"] == nil || got.Upstreams["primary"].Proxy != "127.0.0.1:1080" {
		t.Fatalf("upstream proxy = %#v, want 127.0.0.1:1080", got.Upstreams["primary"])
	}
}
