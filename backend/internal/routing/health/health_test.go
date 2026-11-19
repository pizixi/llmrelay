package health

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llmrelay/backend/internal/domain"
)

func TestTargetCooldownRecoveryAndStaleOutcomes(t *testing.T) {
	now := time.Now()
	health := tracker{upstreams: make(map[[32]byte]map[modelKey]*keyState), now: func() time.Time { return now }}
	policy := domain.ModelRoutingConfig{FailureThreshold: 1, CooldownSeconds: 180}
	upstream := &domain.UpstreamConfig{BaseURL: "https://upstream.example"}
	target := NewTarget("primary", "model-a", upstream)
	failure := ClassifyFailure(503, nil)
	success := Failure{}
	first, _ := health.tryKey(target, "old-key", policy)
	stale, _ := health.tryKey(target, "old-key", policy)
	if delay := first.Complete(&failure); delay != 3*time.Minute {
		t.Fatalf("cooldown = %s", delay)
	}
	stale.Complete(&success)
	if attempt, delay := health.tryKey(target, "old-key", policy); attempt != nil || delay != 3*time.Minute {
		t.Fatalf("cooling target admitted, attempt=%#v delay=%s", attempt, delay)
	}
	otherModel := NewTarget("primary", "model-b", upstream)
	if attempt, _ := health.tryKey(otherModel, "old-key", policy); attempt == nil {
		t.Fatal("cooldown leaked to another model")
	}
	if attempt, _ := health.tryKey(target, "new-key", policy); attempt == nil {
		t.Fatal("other credential inherited old health")
	}
	if available, delay := health.available(target, []string{"old-key", "new-key"}); !available || delay != 0 {
		t.Fatalf("target with a healthy key unavailable, delay=%v", delay)
	}
	statuses := health.statuses(target.upstream, []string{"new-key", "old-key"})[target.model]
	if len(statuses) != 1 || statuses[0] != (KeyStatus{Slot: 2, State: "cooling", Failures: 1, RemainingMS: 180000}) {
		t.Fatalf("status did not follow the credential to its new slot: %#v", statuses)
	}
	now = now.Add(3 * time.Minute)
	if statuses := health.statuses(target.upstream, []string{"old-key"})[target.model]; len(statuses) != 1 || statuses[0].State != "ready_for_probe" {
		t.Fatalf("expired cooldown status=%#v", statuses)
	}
	var admitted atomic.Int32
	var group sync.WaitGroup
	var probe *KeyAttempt
	for range 20 {
		group.Go(func() {
			if attempt, _ := health.tryKey(target, "old-key", policy); attempt != nil {
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
	probe, _ = health.tryKey(target, "old-key", policy)
	if probe == nil {
		t.Fatal("canceled recovery probe remained locked")
	}
	canceledProbe.Complete(nil)
	if second, _ := health.tryKey(target, "old-key", policy); second != nil {
		t.Fatal("completing a canceled probe twice released another probe")
	}
	probe.Complete(&success)
	if attempt, _ := health.tryKey(target, "old-key", policy); attempt == nil {
		t.Fatal("recovered target remained unavailable")
	}
}

func TestCooldownThresholdAndRateLimits(t *testing.T) {
	health := tracker{upstreams: make(map[[32]byte]map[modelKey]*keyState), now: time.Now}
	policy := domain.ModelRoutingConfig{FailureThreshold: 3, CooldownSeconds: 180}
	target := NewTarget("primary", "model", nil)
	failed := ClassifyFailure(500, nil)
	limited := ClassifyFailure(429, nil)
	for _, outcome := range []Failure{failed, failed, limited, failed, failed} {
		attempt, _ := health.tryKey(target, "", policy)
		if attempt == nil {
			t.Fatal("target cooled before three consecutive model faults")
		}
		if delay := attempt.Complete(&outcome); delay != 0 {
			t.Fatalf("unexpected cooldown %s for %#v", delay, outcome)
		}
	}
	attempt, _ := health.tryKey(target, "", policy)
	if delay := attempt.Complete(&failed); delay != 3*time.Minute {
		t.Fatalf("cooldown = %s, want 3 minutes after three consecutive model faults", delay)
	}
	policy.DisableCooldown = true
	if attempt, _ := health.tryKey(target, "", policy); attempt == nil {
		t.Fatal("disabled cooldown still excluded target")
	}
}

func TestKeyCooldownDoublesUntilRecovery(t *testing.T) {
	now := time.Now()
	health := tracker{upstreams: make(map[[32]byte]map[modelKey]*keyState), now: func() time.Time { return now }}
	policy := domain.ModelRoutingConfig{FailureThreshold: 2, CooldownSeconds: 60}
	target := NewTarget("primary", "model-a", nil)
	failed := ClassifyFailure(503, nil)
	fail := func(key string, want time.Duration) {
		t.Helper()
		attempt, _ := health.tryKey(target, key, policy)
		if attempt == nil {
			t.Fatal("eligible key could not be attempted")
		}
		if got := attempt.Complete(&failed); got != want {
			t.Fatalf("cooldown = %s, want %s", got, want)
		}
	}
	fail("first-key", 0)
	for _, delay := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute} {
		fail("first-key", delay)
		if available, wait := health.available(target, []string{"first-key"}); available || wait != delay {
			t.Fatalf("available=%t wait=%s, want %s", available, wait, delay)
		}
		statuses := health.statuses(target.upstream, []string{"first-key"})[target.model]
		if len(statuses) != 1 || statuses[0].RemainingMS != delay.Milliseconds() {
			t.Fatalf("reported cooldown=%#v, want %s", statuses, delay)
		}
		now = now.Add(delay)
		probe, _ := health.tryKey(target, "first-key", policy)
		if probe == nil {
			t.Fatal("expected a recovery probe after cooldown")
		}
		probe.Complete(nil)
	}
	fail("other-key", 0)
	fail("other-key", time.Minute)
	recovered, _ := health.tryKey(target, "first-key", policy)
	if recovered == nil {
		t.Fatal("expected a recovery probe")
	}
	recovered.Complete(&Failure{})
	fail("first-key", 0)
	fail("first-key", time.Minute)
}

func TestKeyCooldownStaysCappedAcrossFailedProbes(t *testing.T) {
	now := time.Now()
	health := tracker{upstreams: make(map[[32]byte]map[modelKey]*keyState), now: func() time.Time { return now }}
	policy := domain.ModelRoutingConfig{FailureThreshold: 1, CooldownSeconds: 6 * 60 * 60}
	target := NewTarget("primary", "model-a", nil)
	failed := ClassifyFailure(503, nil)
	for _, want := range []time.Duration{6 * time.Hour, 12 * time.Hour, 24 * time.Hour, 24 * time.Hour, 24 * time.Hour} {
		attempt, _ := health.tryKey(target, "key", policy)
		if attempt == nil {
			t.Fatal("expected an eligible key")
		}
		if got := attempt.Complete(&failed); got != want {
			t.Fatalf("cooldown = %s, want %s", got, want)
		}
		now = now.Add(want + time.Minute)
	}
}
