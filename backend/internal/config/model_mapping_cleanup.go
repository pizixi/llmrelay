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

// ReconcileRemovedUpstreamModels migrates model alias targets when an upstream
// keeps its relational ID but changes its display name, then removes targets
// whose upstream was deleted or whose model was explicitly removed from that
// upstream's saved custom_models list. An alias with no remaining target is
// removed too.
//
// A model that was not in the previous explicit list is left untouched. This
// preserves manually routed models when custom_models was previously empty
// (which means unrestricted) or when the route was intentionally independent
// of the displayed whitelist.
func ReconcileRemovedUpstreamModels(previous AppConfig, next *AppConfig) ModelMappingCleanup {
	cleanup := ModelMappingCleanup{}
	if next == nil {
		return cleanup
	}

	previousUpstreams := upstreamsByTrimmedName(previous.Upstreams)
	nextUpstreams := upstreamsByTrimmedName(next.Upstreams)
	renamedUpstreams, previousNameByCurrent := detectUpstreamRenames(previousUpstreams, nextUpstreams)
	migrateModelAliasUpstreams(next.ModelAlias, renamedUpstreams)
	migrateUpstreamReferences(next, renamedUpstreams)
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
		if renamed, renamedOK := renamedUpstreams[upstreamName]; renamedOK {
			upstreamName = renamed
		}
		nextUpstream, exists := nextUpstreams[upstreamName]
		if !exists || nextUpstream == nil {
			// Only treat this as a deletion when the upstream existed in the
			// saved configuration. A newly submitted typo must still reach
			// ValidateConfig and produce an explicit unknown-upstream error.
			return previousUpstreams[upstreamName] != nil
		}
		previousUpstreamName := upstreamName
		if previousName, renamed := previousNameByCurrent[upstreamName]; renamed {
			previousUpstreamName = previousName
		}
		previousUpstream := previousUpstreams[previousUpstreamName]
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

// detectUpstreamRenames uses the stable relational ID first. The fingerprint
// fallback keeps configurations submitted by older admin clients safe when
// they do not send the newly exposed ID field: a rename is inferred only when
// exactly one unmatched upstream on each side has the same connection
// identity.
func detectUpstreamRenames(previous, next map[string]*UpstreamConfig) (map[string]string, map[string]string) {
	renamed := make(map[string]string)
	previousNameByCurrent := make(map[string]string)
	previousByID := make(map[int64]string)
	matchedPrevious := make(map[string]struct{})
	matchedNext := make(map[string]struct{})

	for name, upstream := range previous {
		if upstream == nil || upstream.ID <= 0 {
			continue
		}
		if _, exists := previousByID[upstream.ID]; !exists {
			previousByID[upstream.ID] = name
		}
	}
	for name, upstream := range next {
		if upstream == nil || upstream.ID <= 0 {
			continue
		}
		previousName := previousByID[upstream.ID]
		if previousName == "" || previousName == name || next[previousName] != nil {
			continue
		}
		renamed[previousName] = name
		previousNameByCurrent[name] = previousName
		matchedPrevious[previousName] = struct{}{}
		matchedNext[name] = struct{}{}
	}

	// Match only unique fingerprints. This deliberately refuses to guess when
	// two upstreams share the same connection settings.
	previousNames := make([]string, 0, len(previous))
	for name := range previous {
		if next[name] == nil {
			if _, matched := matchedPrevious[name]; !matched {
				previousNames = append(previousNames, name)
			}
		}
	}
	nextNames := make([]string, 0, len(next))
	for name := range next {
		if previous[name] == nil {
			if _, matched := matchedNext[name]; !matched {
				nextNames = append(nextNames, name)
			}
		}
	}
	sort.Strings(previousNames)
	sort.Strings(nextNames)

	candidateOwners := make(map[string][]string)
	previousCandidates := make(map[string][]string)
	for _, previousName := range previousNames {
		for _, nextName := range nextNames {
			if sameUpstreamIdentity(previous[previousName], next[nextName]) {
				candidateOwners[nextName] = append(candidateOwners[nextName], previousName)
				previousCandidates[previousName] = append(previousCandidates[previousName], nextName)
			}
		}
	}
	for nextName, owners := range candidateOwners {
		if len(owners) != 1 {
			continue
		}
		previousName := owners[0]
		if len(previousCandidates[previousName]) != 1 {
			continue
		}
		if _, exists := renamed[previousName]; exists {
			continue
		}
		renamed[previousName] = nextName
		previousNameByCurrent[nextName] = previousName
	}
	return renamed, previousNameByCurrent
}

func sameUpstreamIdentity(previous, next *UpstreamConfig) bool {
	if previous == nil || next == nil {
		return false
	}
	previousAPIType := strings.ToLower(strings.TrimSpace(string(previous.APIType)))
	nextAPIType := strings.ToLower(strings.TrimSpace(string(next.APIType)))
	if previousAPIType == "" {
		previousAPIType = string(UpstreamOpenAI)
	}
	if nextAPIType == "" {
		nextAPIType = string(UpstreamOpenAI)
	}
	previousBridgeMode := strings.ToLower(strings.TrimSpace(string(previous.BridgeMode)))
	nextBridgeMode := strings.ToLower(strings.TrimSpace(string(next.BridgeMode)))
	if previousBridgeMode == "" {
		previousBridgeMode = string(BridgeModeCompatible)
	}
	if nextBridgeMode == "" {
		nextBridgeMode = string(BridgeModeCompatible)
	}
	if strings.TrimSpace(previous.BaseURL) != strings.TrimSpace(next.BaseURL) ||
		strings.TrimSpace(previous.APIKey) != strings.TrimSpace(next.APIKey) ||
		previousAPIType != nextAPIType ||
		previousBridgeMode != nextBridgeMode ||
		strings.TrimSpace(previous.ResponsesReasoningFormat) != strings.TrimSpace(next.ResponsesReasoningFormat) {
		return false
	}
	if previous.MaxRetries == nil || next.MaxRetries == nil {
		return previous.MaxRetries == nil && next.MaxRetries == nil
	}
	return *previous.MaxRetries == *next.MaxRetries
}

func migrateModelAliasUpstreams(aliases map[string]ModelAlias, renamed map[string]string) {
	if len(renamed) == 0 {
		return
	}
	for aliasName, alias := range aliases {
		changed := false
		if len(alias.Targets) == 0 {
			upstreamName := strings.TrimSpace(alias.Upstream)
			if nextName, exists := renamed[upstreamName]; exists {
				alias.Upstream = nextName
				changed = true
			}
		} else {
			targets := make([]ModelAliasTarget, 0, len(alias.Targets))
			seen := make(map[string]struct{}, len(alias.Targets))
			for _, target := range alias.Targets {
				upstreamName := strings.TrimSpace(target.Upstream)
				if nextName, exists := renamed[upstreamName]; exists {
					target.Upstream = nextName
					changed = true
				}
				identity := strings.TrimSpace(target.Upstream) + "\x00" + strings.TrimSpace(target.TargetModel)
				if _, exists := seen[identity]; exists {
					changed = true
					continue
				}
				seen[identity] = struct{}{}
				targets = append(targets, target)
			}
			if changed {
				alias.Targets = targets
			}
		}
		if changed {
			aliases[aliasName] = alias
		}
	}
}

func migrateUpstreamReferences(next *AppConfig, renamed map[string]string) {
	if next == nil || len(renamed) == 0 {
		return
	}
	if nextName, exists := renamed[strings.TrimSpace(next.DefaultUpstream)]; exists {
		next.DefaultUpstream = nextName
	}
	for index, name := range next.UpstreamOrder {
		if nextName, exists := renamed[strings.TrimSpace(name)]; exists {
			next.UpstreamOrder[index] = nextName
		}
	}
}
