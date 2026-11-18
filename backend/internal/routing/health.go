package routing

import (
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"

	"llmrelay/backend/internal/config"
)

type targetState struct {
	failures   int
	until      time.Time
	probing    bool
	generation uint64
	lastUsed   time.Time
}

type healthTracker struct {
	sync.Mutex
	states     map[[32]byte]*targetState
	now        func() time.Time
	lastPruned time.Time
}

var modelHealth = healthTracker{states: make(map[[32]byte]*targetState), now: time.Now}

// TargetAttempt owns a recovery probe when a target's cooldown has expired.
// Complete must be called even when the client cancels its request.
type TargetAttempt struct {
	tracker    *healthTracker
	state      *targetState
	generation uint64
	policy     config.ModelRoutingConfig
	completed  bool
}

func TryTarget(route config.ModelRoute, policy config.ModelRoutingConfig) (*TargetAttempt, time.Duration) {
	return modelHealth.tryTarget(route, policy)
}

// TargetAvailable allows selection to balance only the currently healthy pool.
// TryTarget checks again when the request actually reaches the selected target.
func TargetAvailable(route config.ModelRoute) bool {
	key := targetHealthKey(route)
	modelHealth.Lock()
	defer modelHealth.Unlock()
	state := modelHealth.states[key]
	return state == nil || (!state.probing && !modelHealth.now().Before(state.until))
}

func targetHealthKey(route config.ModelRoute) [32]byte {
	identity, _ := json.Marshal(struct {
		Name     string
		Model    string
		Upstream *config.UpstreamConfig
	}{route.UpstreamName, route.Model, route.Upstream})
	return sha256.Sum256(identity)
}

func (tracker *healthTracker) tryTarget(route config.ModelRoute, policy config.ModelRoutingConfig) (*TargetAttempt, time.Duration) {
	if policy.DisableCooldown {
		return &TargetAttempt{}, 0
	}
	// A changed endpoint, credential, protocol or proxy starts with fresh health.
	// The digest stays internal and never exposes credentials in logs or keys.
	key := targetHealthKey(route)
	tracker.Lock()
	defer tracker.Unlock()
	now := tracker.now()
	if now.Sub(tracker.lastPruned) >= time.Minute {
		for key, state := range tracker.states {
			if !state.probing && now.Sub(state.lastUsed) > 24*time.Hour {
				delete(tracker.states, key)
			}
		}
		tracker.lastPruned = now
	}
	state := tracker.states[key]
	if state == nil {
		state = &targetState{}
		tracker.states[key] = state
	}
	state.lastUsed = now
	if now.Before(state.until) {
		return nil, state.until.Sub(now)
	}
	if state.probing {
		return nil, time.Second
	}
	if !state.until.IsZero() {
		state.probing = true
	}
	return &TargetAttempt{tracker: tracker, state: state, generation: state.generation, policy: policy}, 0
}

// Complete ignores stale concurrent outcomes once a circuit has changed state.
// A nil outcome releases a canceled probe without changing model health.
func (attempt *TargetAttempt) Complete(outcome *Failure) time.Duration {
	if attempt == nil || attempt.tracker == nil {
		return 0
	}
	tracker := attempt.tracker
	tracker.Lock()
	defer tracker.Unlock()
	if attempt.completed {
		return 0
	}
	attempt.completed = true
	state := attempt.state
	if state.generation != attempt.generation {
		return 0
	}
	state.probing = false
	if outcome == nil {
		return 0
	}
	if !outcome.Cooldown {
		state.failures = 0
		state.until = time.Time{}
		state.generation++
		return 0
	}
	state.failures++
	if state.failures < attempt.policy.FailureThreshold {
		return 0
	}
	delay := time.Duration(attempt.policy.CooldownSeconds) * time.Second
	state.until = tracker.now().Add(delay)
	state.generation++
	return delay
}
