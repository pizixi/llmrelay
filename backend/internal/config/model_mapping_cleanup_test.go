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

func TestReconcileDeletedUpstreamKeepsInactiveRemainingAlias(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", CustomModels: []string{"standby-model"}},
			"backup":  {BaseURL: "https://backup.example/v1", CustomModels: []string{"active-model"}},
		},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", CustomModels: []string{"standby-model"}},
		},
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{
				{Upstream: "primary", TargetModel: "standby-model", Weight: 0},
				{Upstream: "backup", TargetModel: "active-model", Weight: 1},
			}},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)
	if cleanup.RemovedTargets != 1 || cleanup.RemovedAliases != 0 {
		t.Fatalf("cleanup = %#v, want one removed target and retained alias", cleanup)
	}
	remaining := next.ModelAlias["chat"].Targets
	if len(remaining) != 1 || remaining[0].Upstream != "primary" || remaining[0].Weight != 0 {
		t.Fatalf("remaining inactive targets = %#v", remaining)
	}
	if err := ValidateConfig(&next); err != nil {
		t.Fatalf("inactive alias left by upstream deletion must remain saveable: %v", err)
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

func TestReconcileRemovedUpstreamModelsMigratesRenamedUpstreamByID(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {ID: 41, BaseURL: "https://primary.example/v1", CustomModels: []string{"model-a", "model-b"}},
		},
		ModelAlias: map[string]ModelAlias{
			"chat": {
				WithReasoning:      true,
				ReasoningEffortMap: map[string]string{"high": "max"},
				Targets: []ModelAliasTarget{{
					Upstream: "primary", TargetModel: "model-a", Weight: 7,
				}},
			},
		},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"main": {ID: 41, BaseURL: "https://primary.example/v1", CustomModels: []string{"model-b"}},
		},
		ModelAlias: map[string]ModelAlias{
			"chat": {
				WithReasoning:      true,
				ReasoningEffortMap: map[string]string{"high": "max"},
				Targets: []ModelAliasTarget{{
					Upstream: "primary", TargetModel: "model-a", Weight: 7,
				}},
			},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 1 || cleanup.RemovedAliases != 1 {
		t.Fatalf("cleanup = %#v, want renamed target to be checked against the new upstream", cleanup)
	}
	if _, exists := next.ModelAlias["chat"]; exists {
		t.Fatal("alias with its only removed model target was not deleted")
	}
}

func TestReconcileRemovedUpstreamModelsPreservesRenamedTargetByID(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {ID: 42, BaseURL: "https://primary.example/v1", CustomModels: []string{"model-a"}},
		},
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{
				Upstream: "primary", TargetModel: "model-a", Weight: 3,
			}}},
		},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"main": {ID: 42, BaseURL: "https://primary.example/v1", CustomModels: []string{"model-a"}},
		},
		UpstreamOrder:   []string{"primary"},
		DefaultUpstream: "primary",
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{
				Upstream: "primary", TargetModel: "model-a", Weight: 3,
			}}},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 0 || cleanup.RemovedAliases != 0 {
		t.Fatalf("cleanup = %#v, rename should not remove the mapping", cleanup)
	}
	if next.DefaultUpstream != "main" || len(next.UpstreamOrder) != 1 || next.UpstreamOrder[0] != "main" {
		t.Fatalf("upstream references were not migrated: default=%q order=%#v", next.DefaultUpstream, next.UpstreamOrder)
	}
	targets := next.ModelAlias["chat"].Targets
	if len(targets) != 1 || targets[0].Upstream != "main" || targets[0].Weight != 3 {
		t.Fatalf("renamed targets = %#v", targets)
	}
}

func TestReconcileRemovedUpstreamModelsInfersUniqueRenameWithoutID(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", APIType: UpstreamOpenAI},
		},
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{
				Upstream: "primary", TargetModel: "model-a", Weight: 1,
			}}},
		},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"main": {BaseURL: "https://primary.example/v1", APIType: UpstreamOpenAI},
		},
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{
				Upstream: "primary", TargetModel: "model-a", Weight: 1,
			}}},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 0 {
		t.Fatalf("cleanup = %#v, unique legacy rename should be preserved", cleanup)
	}
	if got := next.ModelAlias["chat"].Targets[0].Upstream; got != "main" {
		t.Fatalf("renamed upstream = %q, want main", got)
	}
}

func TestReconcileRemovedUpstreamModelsDoesNotGuessAmbiguousRename(t *testing.T) {
	previous := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://same.example/v1", APIType: UpstreamOpenAI},
			"backup":  {BaseURL: "https://same.example/v1", APIType: UpstreamOpenAI},
		},
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{
				Upstream: "primary", TargetModel: "model-a", Weight: 1,
			}}},
		},
	}
	next := AppConfig{
		Upstreams: map[string]*UpstreamConfig{
			"main":    {BaseURL: "https://same.example/v1", APIType: UpstreamOpenAI},
			"backup2": {BaseURL: "https://same.example/v1", APIType: UpstreamOpenAI},
		},
		ModelAlias: map[string]ModelAlias{
			"chat": {Targets: []ModelAliasTarget{{
				Upstream: "primary", TargetModel: "model-a", Weight: 1,
			}}},
		},
	}

	cleanup := ReconcileRemovedUpstreamModels(previous, &next)

	if cleanup.RemovedTargets != 1 {
		t.Fatalf("cleanup = %#v, ambiguous rename should remain a deletion", cleanup)
	}
}
