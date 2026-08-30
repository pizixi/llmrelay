package config

import (
	"context"
	"strings"
	"sync"
)

// trackedRequestContext is attached to one in-flight request for a requested
// model.  Keeping the cancel function here lets ApplyConfig terminate requests
// that were admitted under a model mapping which has since changed.
type trackedRequestContext struct {
	cancel context.CancelFunc
}

var modelRequestContexts = struct {
	sync.Mutex
	byModel map[string]map[*trackedRequestContext]struct{}
}{byModel: make(map[string]map[*trackedRequestContext]struct{})}

// TrackModelRequestContext returns a child context which is canceled when the
// requested model's routing mapping changes.  The release function must be
// called when the request finishes; it is safe to call it more than once.
//
// A child context is returned even for an empty model so callers can uniformly
// replace the request context before validation errors are written.
func TrackModelRequestContext(parent context.Context, model string) (context.Context, func()) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	key := strings.TrimSpace(model)
	if key == "" {
		return ctx, cancel
	}

	tracked := &trackedRequestContext{cancel: cancel}
	modelRequestContexts.Lock()
	requests := modelRequestContexts.byModel[key]
	if requests == nil {
		requests = make(map[*trackedRequestContext]struct{})
		modelRequestContexts.byModel[key] = requests
	}
	requests[tracked] = struct{}{}
	modelRequestContexts.Unlock()

	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			cancel()
			modelRequestContexts.Lock()
			if requests := modelRequestContexts.byModel[key]; requests != nil {
				delete(requests, tracked)
				if len(requests) == 0 {
					delete(modelRequestContexts.byModel, key)
				}
			}
			modelRequestContexts.Unlock()
		})
	}
	return ctx, release
}

// takeModelRequestContexts removes and returns matching cancel functions. It
// is deliberately separate from cancellation so ApplyConfig can detach the
// old requests while holding configMu, then invoke arbitrary context cleanup
// callbacks after releasing the configuration lock.
func takeModelRequestContexts(models map[string]struct{}, cancelAll bool) []context.CancelFunc {
	modelRequestContexts.Lock()
	var cancels []context.CancelFunc
	if cancelAll {
		for key, requests := range modelRequestContexts.byModel {
			for tracked := range requests {
				cancels = append(cancels, tracked.cancel)
			}
			delete(modelRequestContexts.byModel, key)
		}
		modelRequestContexts.Unlock()
		return cancels
	}
	for key := range models {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		requests := modelRequestContexts.byModel[key]
		for tracked := range requests {
			cancels = append(cancels, tracked.cancel)
		}
		delete(modelRequestContexts.byModel, key)
	}
	modelRequestContexts.Unlock()
	return cancels
}
