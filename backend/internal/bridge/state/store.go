// Package state provides the bounded, process-local state used when a
// Responses request is bridged to a protocol that has no native response
// store. It intentionally stores only JSON response objects and never claims
// to be equivalent to a provider's durable conversation store.
package state

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxEntries = 2048
	DefaultTTL        = 30 * time.Minute
)

type entry struct {
	response map[string]any
	storedAt time.Time
	sequence uint64
}

type Store struct {
	mu         sync.Mutex
	entries    map[string]entry
	maxEntries int
	ttl        time.Duration
	sequence   uint64
}

func New(maxEntries int, ttl time.Duration) *Store {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{entries: make(map[string]entry), maxEntries: maxEntries, ttl: ttl}
}

var defaultStore = New(DefaultMaxEntries, DefaultTTL)

func Default() *Store { return defaultStore }

// PutResponse stores a response under its protocol id. It returns false when
// the object has no non-empty string id; callers can then report a precise
// storage downgrade instead of manufacturing an id the provider never sent.
func (s *Store) PutResponse(response map[string]any) (string, bool) {
	if s == nil || response == nil {
		return "", false
	}
	id, _ := response["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	clone, ok := cloneMap(response)
	if !ok {
		return "", false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	s.sequence++
	s.entries[id] = entry{response: clone, storedAt: now, sequence: s.sequence}
	s.pruneLocked(now)
	return id, true
}

func (s *Store) PutResponseBytes(body []byte) (string, bool) {
	var response map[string]any
	if json.Unmarshal(body, &response) != nil {
		return "", false
	}
	return s.PutResponse(response)
}

func (s *Store) Get(id string) (map[string]any, bool) {
	if s == nil {
		return nil, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	value, ok := s.entries[id]
	if !ok {
		return nil, false
	}
	clone, ok := cloneMap(value.response)
	return clone, ok
}

// OutputItems returns assistant/tool items that can be prepended to a later
// Responses input. It does not expose the store's internal map.
func OutputItems(response map[string]any) []any {
	if response == nil {
		return nil
	}
	raw, ok := response["output"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	result := make([]any, 0, len(raw))
	for _, item := range raw {
		if cloned, ok := cloneValue(item); ok {
			result = append(result, cloned)
		}
	}
	return result
}

func (s *Store) ResolveOutputItems(id string) ([]any, bool) {
	response, ok := s.Get(id)
	if !ok {
		return nil, false
	}
	items := OutputItems(response)
	return items, len(items) > 0
}

func (s *Store) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.entries = make(map[string]entry)
	s.sequence = 0
	s.mu.Unlock()
}

func (s *Store) pruneLocked(now time.Time) {
	if s == nil {
		return
	}
	for id, value := range s.entries {
		if now.Sub(value.storedAt) >= s.ttl {
			delete(s.entries, id)
		}
	}
	for len(s.entries) > s.maxEntries {
		oldestID := ""
		var oldest uint64
		for id, value := range s.entries {
			if oldestID == "" || value.sequence < oldest {
				oldestID, oldest = id, value.sequence
			}
		}
		if oldestID == "" {
			return
		}
		delete(s.entries, oldestID)
	}
}

func cloneMap(value map[string]any) (map[string]any, bool) {
	cloned, ok := cloneValue(value)
	if !ok {
		return nil, false
	}
	result, ok := cloned.(map[string]any)
	return result, ok
}

func cloneValue(value any) (any, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	var cloned any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, false
	}
	return cloned, true
}
