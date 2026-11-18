package routing

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llmrelay/backend/internal/config"
)

func TestTargetCooldownRecoveryAndStaleOutcomes(t *testing.T) {
	now := time.Now()
	tracker := healthTracker{states: make(map[[32]byte]*targetState), now: func() time.Time { return now }}
	policy := config.NormalizeModelRoutingConfig(config.ModelRoutingConfig{FailureThreshold: 1})
	route := config.ModelRoute{Model: "model-a", UpstreamName: "primary", Upstream: &config.UpstreamConfig{BaseURL: "https://upstream.example", APIKey: "old-key"}}
	failure := ClassifyFailure(503, nil)
	success := Failure{}
	first, _ := tracker.tryTarget(route, policy)
	stale, _ := tracker.tryTarget(route, policy)
	if delay := first.Complete(&failure); delay != 3*time.Minute {
		t.Fatalf("cooldown = %s", delay)
	}
	stale.Complete(&success)
	if attempt, delay := tracker.tryTarget(route, policy); attempt != nil || delay != 3*time.Minute {
		t.Fatalf("cooling target admitted, attempt=%#v delay=%s", attempt, delay)
	}
	otherModel := route
	otherModel.Model = "model-b"
	if attempt, _ := tracker.tryTarget(otherModel, policy); attempt == nil {
		t.Fatal("cooldown leaked to another model")
	}
	changedKey := route
	changedKey.Upstream = &config.UpstreamConfig{BaseURL: route.Upstream.BaseURL, APIKey: "new-key"}
	if attempt, _ := tracker.tryTarget(changedKey, policy); attempt == nil {
		t.Fatal("replaced credentials inherited old health")
	}
	now = now.Add(3 * time.Minute)
	var admitted atomic.Int32
	var group sync.WaitGroup
	var probe *TargetAttempt
	for range 20 {
		group.Go(func() {
			if attempt, _ := tracker.tryTarget(route, policy); attempt != nil {
				admitted.Add(1)
				probe = attempt
			}
		})
	}
	group.Wait()
	if admitted.Load() != 1 {
		t.Fatalf("admitted recovery probes = %d", admitted.Load())
	}
	probe.Complete(nil)
	canceledProbe := probe
	probe, _ = tracker.tryTarget(route, policy)
	if probe == nil {
		t.Fatal("canceled recovery probe remained locked")
	}
	canceledProbe.Complete(nil)
	if second, _ := tracker.tryTarget(route, policy); second != nil {
		t.Fatal("completing a canceled probe twice released another probe")
	}
	probe.Complete(&success)
	if attempt, _ := tracker.tryTarget(route, policy); attempt == nil {
		t.Fatal("recovered target remained unavailable")
	}
}

func TestCooldownThresholdAndRateLimits(t *testing.T) {
	tracker := healthTracker{states: make(map[[32]byte]*targetState), now: time.Now}
	policy := config.NormalizeModelRoutingConfig(config.ModelRoutingConfig{})
	route := config.ModelRoute{Model: "model", UpstreamName: "primary"}
	failed := ClassifyFailure(500, nil)
	limited := ClassifyFailure(429, nil)
	for _, outcome := range []Failure{failed, failed, limited, failed, failed} {
		attempt, _ := tracker.tryTarget(route, policy)
		if attempt == nil {
			t.Fatal("target cooled before three consecutive model faults")
		}
		if delay := attempt.Complete(&outcome); delay != 0 {
			t.Fatalf("unexpected cooldown %s for %#v", delay, outcome)
		}
	}
	attempt, _ := tracker.tryTarget(route, policy)
	if delay := attempt.Complete(&failed); delay != 3*time.Minute {
		t.Fatalf("cooldown = %s, want 3 minutes after three consecutive model faults", delay)
	}
	policy.DisableCooldown = true
	if attempt, _ := tracker.tryTarget(route, policy); attempt == nil {
		t.Fatal("disabled cooldown still excluded target")
	}
}
