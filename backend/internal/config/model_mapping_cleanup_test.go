package config

import "testing"

func TestReconcileRemovedUpstreamModelsPrunesTargetsAndEmptyAliases(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {CustomModels: []string{"model-a", "model-b"}},
			"backup":  {CustomModels: []string{"model-a"}},
		},
		DefaultUpstream: "primary",
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {CustomModels: []string{"model-b"}},
			"backup":  {CustomModels: []string{"model-a"}},
		},
		DefaultUpstream: "primary",
		ModelAlias: map[string]ModelAlias{
			"multi": {Targets: []ModelAliasTarget{
				{Upstream: "primary", TargetModel: "model-a", Weight: 2},
				{Upstream: "primary", TargetModel: "model-b", Weight: 1},
				{Upstream: "backup", TargetModel: "model-a", Weight: 1},
			}},
			"only-removed": {Targets: []ModelAliasTarget{
				{Upstream: "primary", TargetModel: "model-a", Weight: 1},
			}},
			"legacy": {Upstream: "primary", TargetModel: "model-a"},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 3 || cleanup.RemovedAliases != 2 {
		t.Fatalf("cleanup = %#v, want 3 removed targets and 2 removed aliases", cleanup)
	}
	if _, exists := next.ModelAlias["only-removed"]; exists {
		t.Fatal("alias with no remaining target was not removed")
	}
	if _, exists := next.ModelAlias["legacy"]; exists {
		t.Fatal("legacy alias with a removed target was not removed")
	}
	remaining := next.ModelAlias["multi"].Targets
	if len(remaining) != 2 || remaining[0].TargetModel != "model-b" || remaining[1].Upstream != "backup" {
		t.Fatalf("remaining targets = %#v", remaining)
	}
}

func TestReconcileRemovedUpstreamModelsPrunesDeletedUpstream(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {CustomModels: []string{"model-a"}},
			"backup":  {CustomModels: []string{"model-b"}},
		},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {CustomModels: []string{"model-a"}},
		},
		ModelAlias: map[string]ModelAlias{
			"mixed": {Targets: []ModelAliasTarget{
				{Upstream: "primary", TargetModel: "model-a", Weight: 1},
				{Upstream: "backup", TargetModel: "manually-routed", Weight: 1},
			}},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 1 || cleanup.RemovedAliases != 0 {
		t.Fatalf("cleanup = %#v", cleanup)
	}
	remaining := next.ModelAlias["mixed"].Targets
	if len(remaining) != 1 || remaining[0].Upstream != "primary" {
		t.Fatalf("remaining targets = %#v", remaining)
	}
}

func TestReconcileRemovedUpstreamModelsPreservesManualRouteFromUnrestrictedList(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{"primary": {CustomModels: nil}},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{"primary": {CustomModels: []string{"listed-model"}}},
		ModelAlias: map[string]ModelAlias{
			"manual": {Targets: []ModelAliasTarget{{Upstream: "primary", TargetModel: "manual-model", Weight: 1}}},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 0 {
		t.Fatalf("cleanup = %#v, manual route should be preserved", cleanup)
	}
	if len(next.ModelAlias["manual"].Targets) != 1 {
		t.Fatal("manual route was removed")
	}
}

func TestReconcileRemovedUpstreamModelsLeavesUnknownNewRouteForValidation(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{"primary": {CustomModels: []string{"model-a"}}},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{"primary": {BaseURL: "https://example.com/v1"}},
		ModelAlias: map[string]ModelAlias{
			"typo": {Targets: []ModelAliasTarget{{Upstream: "primray", TargetModel: "model-a", Weight: 1}}},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 0 || len(next.ModelAlias["typo"].Targets) != 1 {
		t.Fatalf("unknown new route was silently removed: cleanup=%#v config=%#v", cleanup, next)
	}
	if err := ValidateConfig(&next); err == nil {
		t.Fatal("unknown new route should still fail validation")
	}
}

func TestReconcileRemovedUpstreamModelsDeletesAliasWhenExplicitListBecomesEmpty(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {CustomModels: []string{"last-model"}},
		},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {CustomModels: []string{}},
		},
		ModelAlias: map[string]ModelAlias{
			"last-alias": {Targets: []ModelAliasTarget{{Upstream: "primary", TargetModel: "last-model", Weight: 1}}},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 1 || cleanup.RemovedAliases != 1 {
		t.Fatalf("cleanup = %#v", cleanup)
	}
	if _, exists := next.ModelAlias["last-alias"]; exists {
		t.Fatal("alias with no remaining target was not deleted")
	}
}
