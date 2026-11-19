package config

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
)

const (
	RoutingPriority           = "priority"
	RoutingWeightedRoundRobin = "weighted_round_robin"
)

func NormalizeModelRoutingConfig(value ModelRoutingConfig) ModelRoutingConfig {
	value.Strategy = strings.ToLower(strings.TrimSpace(value.Strategy))
	if value.Strategy == "" {
		value.Strategy = RoutingPriority
	}
	if value.FailureThreshold == 0 {
		value.FailureThreshold = 3
	}
	if value.CooldownSeconds == 0 {
		value.CooldownSeconds = 180
	}
	return value
}

func ValidateModelRoutingConfig(value ModelRoutingConfig) error {
	value = NormalizeModelRoutingConfig(value)
	if value.Strategy != RoutingPriority && value.Strategy != RoutingWeightedRoundRobin {
		return fmt.Errorf("model_routing.strategy must be priority or weighted_round_robin")
	}
	if value.MaxAttempts < 0 || value.MaxAttempts > 1000 {
		return fmt.Errorf("model_routing.max_attempts must be between 0 and 1000 (0 tries every target)")
	}
	if value.FailureThreshold < 1 || value.FailureThreshold > 100 {
		return fmt.Errorf("model_routing.failure_threshold must be between 1 and 100")
	}
	if value.CooldownSeconds < 1 || value.CooldownSeconds > 86400 {
		return fmt.Errorf("model_routing.cooldown_seconds must be between 1 and 86400")
	}
	return nil
}

// ModelRoute is an immutable snapshot of one allowed upstream/model target.
type ModelRoute struct {
	Model        string
	Alias        ModelAlias
	AliasMatched bool
	UpstreamName string
	Upstream     *UpstreamConfig
}

// ResolveRequestModelCandidates returns each target once in request order.
// Alias failover is limited to the alias's enabled targets. Direct models keep
// their existing round-robin starting point and try the remaining matches.
// When balancing multiple targets, the optional availability callback excludes
// cooling or busy recovery targets while retaining them in the result.
func ResolveRequestModelCandidates(model string, availability ...func(ModelRoute) bool) ([]ModelRoute, bool, ModelRoutingConfig) {
	model = strings.TrimSpace(model)
	configMu.RLock()
	defer configMu.RUnlock()
	policy := modelRouting
	isAvailable := func(route ModelRoute) bool {
		return policy.DisableCooldown || len(availability) == 0 || availability[0](route)
	}
	if alias, matched := modelAlias[model]; matched {
		alias = CloneModelAlias(alias)
		if len(alias.Targets) == 0 {
			resolved := alias.TargetModel
			if resolved == "" {
				resolved = model
			}
			name, upstream := resolveUpstreamLocked(alias.Upstream)
			return []ModelRoute{{Model: resolved, Alias: alias, AliasMatched: true, UpstreamName: name, Upstream: upstream}}, true, policy
		}
		targets := make([]ModelAliasTarget, 0, len(alias.Targets))
		for _, target := range alias.Targets {
			if target.Enabled && target.Weight > 0 {
				targets = append(targets, target)
			}
		}
		if len(targets) == 0 {
			return []ModelRoute{{Model: model, Alias: alias, AliasMatched: true}}, false, policy
		}
		if len(targets) > 1 {
			var first ModelAliasTarget
			targetAvailable := func(target ModelAliasTarget) bool {
				if policy.DisableCooldown || len(availability) == 0 {
					return true
				}
				return isAvailable(ModelRoute{Model: target.TargetModel, UpstreamName: target.Upstream, Upstream: CloneUpstreamConfig(upstreamCfgs[target.Upstream])})
			}
			if policy.Strategy == RoutingWeightedRoundRobin {
				var availableTargets []ModelAliasTarget
				for _, target := range targets {
					if targetAvailable(target) {
						availableTargets = append(availableTargets, target)
					}
				}
				if len(availableTargets) > 0 {
					value, _ := aliasTargetCounters.LoadOrStore(model, &atomic.Uint64{})
					first = SelectWeightedAliasTarget(availableTargets, value.(*atomic.Uint64).Add(1)-1)
				}
			}
			sort.SliceStable(targets, func(i, j int) bool { return targets[i].Weight > targets[j].Weight })
			rotateEqualWeightTargets(model, targets, targetAvailable)
			if first.Upstream != "" {
				for index, target := range targets {
					if target == first {
						copy(targets[1:index+1], targets[:index])
						targets[0] = first
						break
					}
				}
			}
		}
		routes := make([]ModelRoute, 0, len(targets))
		for _, target := range targets {
			selectedAlias := alias
			selectedAlias.TargetModel = target.TargetModel
			selectedAlias.Upstream = target.Upstream
			routes = append(routes, ModelRoute{Model: target.TargetModel, Alias: selectedAlias, AliasMatched: true,
				UpstreamName: target.Upstream, Upstream: CloneUpstreamConfig(upstreamCfgs[target.Upstream])})
		}
		return routes, true, policy
	}
	var routes []ModelRoute
	if allowDirectModels {
		for _, name := range NormalizeUpstreamOrder(upstreamOrder, upstreamCfgs) {
			if upstreamHasModel(upstreamCfgs[name], model) {
				routes = append(routes, ModelRoute{Model: model, UpstreamName: name, Upstream: CloneUpstreamConfig(upstreamCfgs[name])})
			}
		}
	}
	if len(routes) == 1 {
		return routes, true, policy
	}
	if len(routes) > 1 {
		var availableRoutes, unavailableRoutes []ModelRoute
		for _, route := range routes {
			if isAvailable(route) {
				availableRoutes = append(availableRoutes, route)
			} else {
				unavailableRoutes = append(unavailableRoutes, route)
			}
		}
		if len(availableRoutes) == 0 {
			return unavailableRoutes, true, policy
		}
		value, _ := modelRouteCounters.LoadOrStore(model, &atomic.Uint64{})
		index := int((value.(*atomic.Uint64).Add(1) - 1) % uint64(len(availableRoutes)))
		ordered := append([]ModelRoute(nil), availableRoutes[index:]...)
		ordered = append(ordered, availableRoutes[:index]...)
		return append(ordered, unavailableRoutes...), true, policy
	}
	if allowDirectModels && legacyDefaultRoute {
		upstream := upstreamCfgs[defaultUpstreamName]
		if upstream != nil && len(upstream.CustomModels) == 0 {
			return []ModelRoute{{Model: model, UpstreamName: defaultUpstreamName, Upstream: CloneUpstreamConfig(upstream)}}, true, policy
		}
	}
	// Retain the previous not-found context for callers of ResolveRequestModel.
	name := ""
	if legacyDefaultRoute {
		name = defaultUpstreamName
	}
	if upstreamCfgs[name] == nil {
		if names := NormalizeUpstreamOrder(upstreamOrder, upstreamCfgs); len(names) > 0 {
			name = names[0]
		}
	}
	return []ModelRoute{{Model: model, UpstreamName: name, Upstream: CloneUpstreamConfig(upstreamCfgs[name])}}, false, policy
}

// Each weight tier has an independent round-robin starting point. Rotating
// every tier also distributes requests that fall through to backup tiers.
func rotateEqualWeightTargets(model string, targets []ModelAliasTarget, available func(ModelAliasTarget) bool) {
	for start := 0; start < len(targets); {
		end := start + 1
		for end < len(targets) && targets[end].Weight == targets[start].Weight {
			end++
		}
		var group, unavailable []ModelAliasTarget
		for _, target := range targets[start:end] {
			if available(target) {
				group = append(group, target)
			} else {
				unavailable = append(unavailable, target)
			}
		}
		if len(group) > 1 {
			key := struct {
				Model  string
				Weight int
			}{model, targets[start].Weight}
			value, _ := aliasTargetCounters.LoadOrStore(key, &atomic.Uint64{})
			offset := int((value.(*atomic.Uint64).Add(1) - 1) % uint64(len(group)))
			ordered := append([]ModelAliasTarget(nil), group[offset:]...)
			ordered = append(ordered, group[:offset]...)
			copy(group, ordered)
		}
		copy(targets[start:end], append(group, unavailable...))
		start = end
	}
}
