package config

import (
	"context"
	"testing"
)

func TestApplyConfigCancelsRequestsForChangedModelMapping(t *testing.T) {
	previous := Snapshot()
	t.Cleanup(func() { ApplyConfig(previous) })

	base := AppConfig{
		ModelAlias: map[string]ModelAlias{
			"chat": {TargetModel: "old-model", Upstream: "primary"},
			"keep": {TargetModel: "keep-model", Upstream: "primary"},
		},
		Upstreams: map[string]*UpstreamConfig{
			"primary": {BaseURL: "https://primary.example/v1", CustomModels: []string{"old-model", "new-model", "keep-model"}},
		},
		UpstreamOrder: []string{"primary"},
	}
	ApplyConfig(base)

	changedCtx, releaseChanged := TrackModelRequestContext(context.Background(), "chat")
	defer releaseChanged()
	unchangedCtx, releaseUnchanged := TrackModelRequestContext(context.Background(), "keep")
	defer releaseUnchanged()

	next := base
	next.ModelAlias = map[string]ModelAlias{
		"chat": {TargetModel: "new-model", Upstream: "primary"},
		"keep": {TargetModel: "keep-model", Upstream: "primary"},
	}
	ApplyConfig(next)

	select {
	case <-changedCtx.Done():
	default:
		t.Fatal("changed model request context was not canceled")
	}
	select {
	case <-unchangedCtx.Done():
		t.Fatal("unchanged model request context was canceled")
	default:
	}
}
