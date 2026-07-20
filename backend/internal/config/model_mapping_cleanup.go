package config

import (
	"sort"
	"strings"
)

// ModelMappingCleanup summarizes alias targets removed while reconciling an
// updated upstream model whitelist with the previously saved configuration.
type ModelMappingCleanup struct {
	RemovedTargets int                        `json:"removed_targets"`
	RemovedAliases int                        `json:"removed_aliases"`
	Aliases        []ModelMappingCleanupAlias `json:"aliases,omitempty"`
}

type ModelMappingCleanupAlias struct {
	Alias          string             `json:"alias"`
	RemovedTargets []ModelAliasTarget `json:"removed_targets"`
	AliasRemoved   bool               `json:"alias_removed"`
}

// ReconcileRemovedUpstreamModels removes model alias targets whose upstream
// was deleted, or whose model was explicitly removed from that upstream's
// saved custom_models list. An alias with no remaining target is removed too.
//
// A model that was not in the previous explicit list is left untouched. This
// preserves manually routed models when custom_models was previously empty
// (which means unrestricted) or when the route was intentionally independent
// of the displayed whitelist.
func ReconcileRemovedUpstreamModels(previous AppConfig, next *AppConfig) ModelMappingCleanup {
	cleanup := ModelMappingCleanup{}
	if next == nil || len(next.ModelAlias) == 0 {
		return cleanup
	}

	previousUpstreams := upstreamsByTrimmedName(previous.Upstreams)
	nextUpstreams := upstreamsByTrimmedName(next.Upstreams)
	previousDefault := strings.TrimSpace(previous.DefaultUpstream)
	nextDefault := strings.TrimSpace(next.DefaultUpstream)

	shouldRemove := func(upstreamName, targetModel string) bool {
		upstreamName = strings.TrimSpace(upstreamName)
		targetModel = strings.TrimSpace(targetModel)
		if targetModel == "" {
			return false
		}
		if upstreamName == "" {
			upstreamName = nextDefault
			if upstreamName == "" {
				upstreamName = previousDefault
			}
		}
		if upstreamName == "" {
			return false
		}
		nextUpstream, exists := nextUpstreams[upstreamName]
		if !exists || nextUpstream == nil {
			// Only treat this as a deletion when the upstream existed in the
			// saved configuration. A newly submitted typo must still reach
			// ValidateConfig and produce an explicit unknown-upstream error.
			return previousUpstreams[upstreamName] != nil
		}
		previousUpstream := previousUpstreams[upstreamName]
		if previousUpstream == nil || !modelListContains(previousUpstream.CustomModels, targetModel) {
			return false
		}
		return !modelListContains(nextUpstream.CustomModels, targetModel)
	}

	for aliasName, alias := range next.ModelAlias {
		removed := make([]ModelAliasTarget, 0)
		aliasRemoved := false
		if len(alias.Targets) == 0 {
			if alias.TargetModel == "" || !shouldRemove(alias.Upstream, alias.TargetModel) {
				continue
			}
			removed = append(removed, ModelAliasTarget{
				TargetModel: strings.TrimSpace(alias.TargetModel),
				Upstream:    strings.TrimSpace(alias.Upstream),
				Weight:      1,
			})
			delete(next.ModelAlias, aliasName)
			aliasRemoved = true
		} else {
			remaining := make([]ModelAliasTarget, 0, len(alias.Targets))
			for _, target := range alias.Targets {
				if shouldRemove(target.Upstream, target.TargetModel) {
					removed = append(removed, target)
					continue
				}
				remaining = append(remaining, target)
			}
			if len(removed) == 0 {
				continue
			}
			if len(remaining) == 0 {
				delete(next.ModelAlias, aliasName)
				aliasRemoved = true
			} else {
				alias.Targets = remaining
				alias.TargetModel = ""
				alias.Upstream = ""
				next.ModelAlias[aliasName] = alias
			}
		}

		cleanup.RemovedTargets += len(removed)
		if aliasRemoved {
			cleanup.RemovedAliases++
		}
		cleanup.Aliases = append(cleanup.Aliases, ModelMappingCleanupAlias{
			Alias:          aliasName,
			RemovedTargets: removed,
			AliasRemoved:   aliasRemoved,
		})
	}

	sort.Slice(cleanup.Aliases, func(i, j int) bool {
		return cleanup.Aliases[i].Alias < cleanup.Aliases[j].Alias
	})
	return cleanup
}

func upstreamsByTrimmedName(source map[string]*UpstreamConfig) map[string]*UpstreamConfig {
	result := make(map[string]*UpstreamConfig, len(source))
	for name, upstream := range source {
		name = strings.TrimSpace(name)
		if name != "" {
			result[name] = upstream
		}
	}
	return result
}

func modelListContains(models []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, model := range models {
		if strings.TrimSpace(model) == target {
			return true
		}
	}
	return false
}
