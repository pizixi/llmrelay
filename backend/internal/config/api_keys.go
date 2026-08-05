package config

import (
	"fmt"
	"strings"
	"time"

	"llmrelay/backend/internal/domain"
)

type APIKey = domain.APIKey

// NormalizeAPIKeys removes incomplete and duplicate entries while filling in
// stable values for keys imported from older configuration files.
func NormalizeAPIKeys(values []APIKey) []APIKey {
	if len(values) == 0 {
		return nil
	}
	result := make([]APIKey, 0, len(values))
	seenIDs := make(map[string]struct{}, len(values))
	seenKeys := make(map[string]struct{}, len(values))
	seenNames := make(map[string]struct{}, len(values))
	for index, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Name = strings.TrimSpace(value.Name)
		value.Key = strings.TrimSpace(value.Key)
		if value.Key == "" {
			continue
		}
		if value.ID == "" {
			value.ID = fmt.Sprintf("key-%d", index+1)
		}
		if value.Name == "" {
			value.Name = "未命名密钥"
		}
		if _, exists := seenIDs[value.ID]; exists {
			continue
		}
		if _, exists := seenKeys[value.Key]; exists {
			continue
		}
		nameKey := strings.ToLower(value.Name)
		if _, exists := seenNames[nameKey]; exists {
			continue
		}
		seenIDs[value.ID] = struct{}{}
		seenKeys[value.Key] = struct{}{}
		seenNames[nameKey] = struct{}{}
		if strings.TrimSpace(value.CreatedAt) == "" {
			value.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		result = append(result, value)
	}
	return result
}

func CloneAPIKeys(values []APIKey) []APIKey {
	if values == nil {
		return nil
	}
	return append([]APIKey(nil), values...)
}

// MigrateLegacyAPIKey moves the old command-line/environment credential into
// the managed key list. It is deliberately idempotent so restarting with the
// same legacy setting does not create additional entries.
func MigrateLegacyAPIKey(cfg *AppConfig, legacyKey string) bool {
	if cfg == nil {
		return false
	}
	legacyKey = strings.TrimSpace(legacyKey)
	if legacyKey == "" {
		return false
	}
	for _, value := range cfg.APIKeys {
		if strings.TrimSpace(value.Key) == legacyKey {
			return false
		}
	}

	name := "默认密钥"
	if len(cfg.APIKeys) > 0 {
		name = "迁移的旧版密钥"
	}
	usedNames := make(map[string]struct{}, len(cfg.APIKeys))
	usedIDs := make(map[string]struct{}, len(cfg.APIKeys))
	for _, value := range cfg.APIKeys {
		usedNames[strings.TrimSpace(value.Name)] = struct{}{}
		usedIDs[strings.TrimSpace(value.ID)] = struct{}{}
	}
	for suffix := 2; ; suffix++ {
		if _, exists := usedNames[name]; !exists {
			break
		}
		name = fmt.Sprintf("迁移的旧版密钥 %d", suffix)
	}
	id := "legacy-default"
	for suffix := 2; ; suffix++ {
		if _, exists := usedIDs[id]; !exists {
			break
		}
		id = fmt.Sprintf("legacy-%d", suffix)
	}
	cfg.APIKeys = append(cfg.APIKeys, APIKey{
		ID:        id,
		Name:      name,
		Key:       legacyKey,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	return true
}
