package health

import (
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"

	"llmrelay/backend/internal/domain"
)

const maxKeyCooldown = 24 * time.Hour

type keyState struct {
	failures   int
	until      time.Time
	cooldown   time.Duration
	probing    bool
	generation uint64
	lastUsed   time.Time
}

type modelKey struct {
	model      string
	credential [32]byte
}

type tracker struct {
	sync.Mutex
	upstreams  map[[32]byte]map[modelKey]*keyState
	now        func() time.Time
	lastPruned time.Time
}

var modelHealth = tracker{upstreams: make(map[[32]byte]map[modelKey]*keyState), now: time.Now}

// Target identifies the upstream and actual model. Each credential has its own
// circuit, shared by aliases pointing to this target.
type Target struct {
	upstream [32]byte
	model    string
}

func NewTarget(name, model string, upstream *domain.UpstreamConfig) Target {
	return Target{upstream: upstreamIdentity(name, upstream), model: model}
}

func upstreamIdentity(name string, upstream *domain.UpstreamConfig) [32]byte {
	identity := struct {
		ID      int64
		Name    string
		BaseURL string
		APIType domain.UpstreamType
		Proxy   string
	}{Name: name}
	if upstream != nil {
		identity.ID = upstream.ID
		identity.BaseURL = upstream.BaseURL
		identity.APIType = upstream.APIType
		identity.Proxy = upstream.Proxy
		if identity.ID != 0 {
			identity.Name = ""
		}
	}
	encoded, _ := json.Marshal(identity)
	return sha256.Sum256(encoded)
}

func (target Target) key(credential string) modelKey {
	return modelKey{model: target.model, credential: sha256.Sum256([]byte(credential))}
}

func (state *keyState) wait(now time.Time) time.Duration {
	if state == nil {
		return 0
	}
	if now.Before(state.until) {
		return state.until.Sub(now)
	}
	if state.probing {
		return time.Second
	}
	return 0
}

func (target Target) Available(keys []string) (bool, time.Duration) {
	return modelHealth.available(target, keys)
}

func (health *tracker) available(target Target, keys []string) (bool, time.Duration) {
	health.Lock()
	defer health.Unlock()
	states, now := health.upstreams[target.upstream], health.now()
	if len(keys) == 0 {
		wait := states[target.key("")].wait(now)
		return wait == 0, wait
	}
	var soonest time.Duration
	for _, credential := range keys {
		wait := states[target.key(credential)].wait(now)
		if wait == 0 {
			return true, 0
		}
		if soonest == 0 || wait < soonest {
			soonest = wait
		}
	}
	return false, soonest
}

// KeyAttempt owns a recovery probe when a credential's cooldown has expired.
// Complete must also be called when the request is canceled.
type KeyAttempt struct {
	tracker    *tracker
	state      *keyState
	generation uint64
	policy     domain.ModelRoutingConfig
	completed  bool
}

func (target Target) TryKey(credential string, policy domain.ModelRoutingConfig) (*KeyAttempt, time.Duration) {
	return modelHealth.tryKey(target, credential, policy)
}

func (health *tracker) tryKey(target Target, credential string, policy domain.ModelRoutingConfig) (*KeyAttempt, time.Duration) {
	if policy.DisableCooldown {
		return &KeyAttempt{}, 0
	}
	key := target.key(credential)
	health.Lock()
	defer health.Unlock()
	now := health.now()
	if now.Sub(health.lastPruned) >= time.Minute {
		// Retain cooldown history through the cooldown and the subsequent probe window.
		for upstream, states := range health.upstreams {
			for key, state := range states {
				if !state.probing && now.Sub(state.lastUsed) > 24*time.Hour && now.Sub(state.until) > 24*time.Hour {
					delete(states, key)
				}
			}
			if len(states) == 0 {
				delete(health.upstreams, upstream)
			}
		}
		health.lastPruned = now
	}
	states := health.upstreams[target.upstream]
	if states == nil {
		states = make(map[modelKey]*keyState)
		health.upstreams[target.upstream] = states
	}
	state := states[key]
	if delay := state.wait(now); delay > 0 {
		return nil, delay
	}
	if state == nil {
		state = &keyState{}
		states[key] = state
	}
	state.lastUsed = now
	state.probing = !state.until.IsZero()
	return &KeyAttempt{tracker: health, state: state, generation: state.generation, policy: policy}, 0
}

// Complete ignores stale outcomes after the circuit opens or recovers. A nil
// outcome releases a probe without changing its health.
func (attempt *KeyAttempt) Complete(outcome *Failure) time.Duration {
	if attempt == nil || attempt.tracker == nil {
		return 0
	}
	health := attempt.tracker
	health.Lock()
	defer health.Unlock()
	if attempt.completed {
		return 0
	}
	attempt.completed = true
	state := attempt.state
	if state.generation != attempt.generation {
		return 0
	}
	state.lastUsed = health.now()
	probing := state.probing
	state.probing = false
	if outcome == nil {
		return 0
	}
	if !outcome.Cooldown {
		state.failures = 0
		state.until = time.Time{}
		state.cooldown = 0
		if probing {
			state.generation++
		}
		return 0
	}
	state.failures++
	if !probing && state.failures < attempt.policy.FailureThreshold {
		return 0
	}
	delay := time.Duration(attempt.policy.CooldownSeconds) * time.Second
	if probing {
		delay = max(delay, state.cooldown*2)
	}
	delay = min(delay, maxKeyCooldown)
	state.cooldown = delay
	state.until = state.lastUsed.Add(delay)
	state.generation++
	return delay
}

// KeyStatus uses the configured slot number, with zero for requests without a key.
type KeyStatus struct {
	Slot        int    `json:"slot"`
	State       string `json:"state"`
	Failures    int    `json:"failures"`
	RemainingMS int64  `json:"remaining_ms,omitempty"`
}

// UpstreamStatuses lists only configured credentials with recorded failures.
func UpstreamStatuses(name string, upstream *domain.UpstreamConfig, keys []string) map[string][]KeyStatus {
	return modelHealth.statuses(upstreamIdentity(name, upstream), keys)
}

func (health *tracker) statuses(upstream [32]byte, keys []string) map[string][]KeyStatus {
	slots := make(map[[32]byte]int, len(keys)+1)
	for index, key := range keys {
		slots[sha256.Sum256([]byte(key))] = index + 1
	}
	if len(keys) == 0 {
		slots[sha256.Sum256(nil)] = 0
	}

	health.Lock()
	defer health.Unlock()
	now := health.now()
	models := make(map[string][]KeyStatus)
	for key, state := range health.upstreams[upstream] {
		slot, configured := slots[key.credential]
		if !configured || state.failures == 0 {
			continue
		}
		status := KeyStatus{Slot: slot, State: "available", Failures: state.failures}
		switch {
		case state.probing:
			status.State = "probing"
		case now.Before(state.until):
			status.State = "cooling"
			status.RemainingMS = (state.until.Sub(now) + time.Millisecond - 1).Milliseconds()
		case !state.until.IsZero():
			status.State = "ready_for_probe"
		}
		models[key.model] = append(models[key.model], status)
	}
	return models
}
