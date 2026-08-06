package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"llmrelay/backend/internal/storage"
)

// 配置使用关系模型持久化。AppConfig 仍然作为运行时和管理 API 的聚合
// 对象存在，但不会再整体序列化到数据库中。
const normalizedConfigSchemaVersion = 1

const (
	configActiveProxyDirect            = "direct"
	configActiveProxyRoundRobin        = "round_robin"
	configActiveProxyRateLimitSwitch   = "rate_limit_switch"
	configActiveProxyRateLimitNoDirect = "rate_limit_switch_no_direct"
	configActiveProxyFixed             = "fixed"
)

const configSchema = `
CREATE TABLE IF NOT EXISTS config_schema_version (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    version INTEGER NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS upstreams (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    base_url TEXT NOT NULL,
    proxy_addr TEXT NOT NULL DEFAULT '',
    api_type TEXT NOT NULL DEFAULT 'openai'
        CHECK (api_type IN ('openai', 'anthropic', 'openai-responses')),
    bridge_mode TEXT NOT NULL DEFAULT 'compatible'
        CHECK (bridge_mode IN ('compatible', 'strict')),
    capabilities_json TEXT NOT NULL DEFAULT '{}',
    responses_reasoning_format TEXT NOT NULL DEFAULT '',
    max_retries INTEGER,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_upstreams_sort_order ON upstreams(sort_order, id);

CREATE TABLE IF NOT EXISTS upstream_api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    upstream_id INTEGER NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
    api_key TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (upstream_id, api_key)
);
CREATE INDEX IF NOT EXISTS idx_upstream_api_keys_upstream ON upstream_api_keys(upstream_id, sort_order, id);

CREATE TABLE IF NOT EXISTS upstream_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    upstream_id INTEGER NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (upstream_id, model_name)
);
CREATE INDEX IF NOT EXISTS idx_upstream_models_upstream ON upstream_models(upstream_id, sort_order, id);

CREATE TABLE IF NOT EXISTS model_aliases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    alias TEXT NOT NULL UNIQUE,
    with_reasoning INTEGER NOT NULL DEFAULT 0 CHECK (with_reasoning IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_model_aliases_sort_order ON model_aliases(sort_order, id);

CREATE TABLE IF NOT EXISTS model_alias_targets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    alias_id INTEGER NOT NULL REFERENCES model_aliases(id) ON DELETE CASCADE,
    upstream_id INTEGER NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
    target_model TEXT NOT NULL,
    weight INTEGER NOT NULL DEFAULT 1 CHECK (weight BETWEEN 0 AND 1000000),
    sort_order INTEGER NOT NULL DEFAULT 0,
    UNIQUE (alias_id, upstream_id, target_model)
);
CREATE INDEX IF NOT EXISTS idx_model_alias_targets_alias ON model_alias_targets(alias_id, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_model_alias_targets_upstream ON model_alias_targets(upstream_id);

CREATE TABLE IF NOT EXISTS global_reasoning_effort_mappings (
    request_effort TEXT PRIMARY KEY,
    mapped_effort TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS alias_reasoning_effort_mappings (
    alias_id INTEGER NOT NULL REFERENCES model_aliases(id) ON DELETE CASCADE,
    request_effort TEXT NOT NULL,
    mapped_effort TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (alias_id, request_effort)
);
CREATE INDEX IF NOT EXISTS idx_alias_reasoning_effort_alias ON alias_reasoning_effort_mappings(alias_id);

CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    secret TEXT NOT NULL UNIQUE,
    disabled INTEGER NOT NULL DEFAULT 0 CHECK (disabled IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_api_keys_enabled ON api_keys(disabled, sort_order, id);

CREATE TABLE IF NOT EXISTS socks5_proxies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    addr TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL DEFAULT '',
    password TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_socks5_proxies_sort_order ON socks5_proxies(sort_order, id);

CREATE TABLE IF NOT EXISTS web_search_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    provider TEXT NOT NULL DEFAULT '',
    fallback_provider TEXT NOT NULL DEFAULT '',
    base_url TEXT NOT NULL DEFAULT '',
    api_key TEXT NOT NULL DEFAULT '',
    searxng_mode TEXT NOT NULL DEFAULT '',
    searxng_directory_url TEXT NOT NULL DEFAULT '',
    max_results INTEGER NOT NULL DEFAULT 0,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    max_tool_rounds INTEGER NOT NULL DEFAULT 0,
    max_result_bytes INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS gateway_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    default_upstream_id INTEGER REFERENCES upstreams(id) ON DELETE SET NULL,
    active_proxy_mode TEXT NOT NULL DEFAULT 'direct'
        CHECK (active_proxy_mode IN ('direct', 'round_robin', 'rate_limit_switch', 'rate_limit_switch_no_direct', 'fixed')),
    active_proxy_id INTEGER REFERENCES socks5_proxies(id) ON DELETE SET NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func ensureConfigSchema(db *sql.DB) error {
	if _, err := db.Exec(configSchema); err != nil {
		return fmt.Errorf("initialize config schema: %w", err)
	}
	var version int
	err := db.QueryRow("SELECT version FROM config_schema_version WHERE id = 1").Scan(&version)
	if err == sql.ErrNoRows {
		if _, err := db.Exec(`INSERT INTO config_schema_version(id, version) VALUES(1, ?)
			ON CONFLICT(id) DO NOTHING`, normalizedConfigSchemaVersion); err != nil {
			return fmt.Errorf("initialize config schema version: %w", err)
		}
		if err := ensureUpstreamCapabilitiesColumn(db); err != nil {
			return err
		}
		return ensureUpstreamProxyColumn(db)
	}
	if err != nil {
		return fmt.Errorf("read config schema version: %w", err)
	}
	if version != normalizedConfigSchemaVersion {
		return fmt.Errorf("unsupported config schema version %d", version)
	}
	if err := ensureUpstreamCapabilitiesColumn(db); err != nil {
		return err
	}
	return ensureUpstreamProxyColumn(db)
}

// ensureUpstreamCapabilitiesColumn keeps databases created by older releases
// readable without a destructive schema reset. SQLite's CREATE TABLE IF NOT
// EXISTS does not add columns to an existing table, so this small idempotent
// migration is required before normalized reads/writes.
func ensureUpstreamCapabilitiesColumn(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(upstreams)")
	if err != nil {
		return fmt.Errorf("inspect upstream capability column: %w", err)
	}
	defer rows.Close()
	hasColumn := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("scan upstream schema: %w", err)
		}
		if name == "capabilities_json" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read upstream schema: %w", err)
	}
	if hasColumn {
		return nil
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close upstream schema rows: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE upstreams ADD COLUMN capabilities_json TEXT NOT NULL DEFAULT '{}'`); err != nil {
		return fmt.Errorf("add upstream capabilities column: %w", err)
	}
	return nil
}

func ensureUpstreamProxyColumn(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(upstreams)")
	if err != nil {
		return fmt.Errorf("inspect upstream proxy column: %w", err)
	}
	defer rows.Close()
	hasColumn := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return fmt.Errorf("scan upstream proxy schema: %w", err)
		}
		if name == "proxy_addr" {
			hasColumn = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read upstream proxy schema: %w", err)
	}
	if hasColumn {
		return nil
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close upstream proxy schema rows: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE upstreams ADD COLUMN proxy_addr TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add upstream proxy column: %w", err)
	}
	return nil
}

func loadSQLiteConfig(path string) (AppConfig, error) {
	var cfg AppConfig
	db, err := storage.Open(path)
	if err != nil {
		return cfg, err
	}
	defer db.Close()
	if err := ensureConfigSchema(db); err != nil {
		return cfg, err
	}
	if err := migrateLegacyConfigTable(db); err != nil {
		return cfg, err
	}
	cfg, err = readNormalizedConfig(db)
	if err != nil {
		return cfg, fmt.Errorf("read sqlite config: %w", err)
	}
	NormalizeConfig(&cfg)
	if err := ValidateConfig(&cfg); err != nil {
		return cfg, fmt.Errorf("validate sqlite config: %w", err)
	}
	return cfg, nil
}

func saveSQLiteConfig(path string, cfg AppConfig) error {
	// SaveConfig is also used by small integrations outside the admin handler;
	// validate here so a direct caller cannot create a partially unusable DB.
	if err := ValidateConfig(&cfg); err != nil {
		return err
	}
	NormalizeConfig(&cfg)
	if err := ValidateConfig(&cfg); err != nil {
		return err
	}
	db, err := storage.Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := ensureConfigSchema(db); err != nil {
		return err
	}
	if err := migrateLegacyConfigTable(db); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin config transaction: %w", err)
	}
	if err := writeNormalizedConfig(tx, cfg); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("save sqlite config: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite config: %w", err)
	}
	return nil
}

// migrateLegacyConfigTable is a one-time migration for the intermediate
// SQLite release that stored the entire AppConfig in app_config.data. It is
// intentionally limited to an existing database row; no JSON file is ever
// inspected. The table is removed in the same transaction as the normalized
// rows, so a failed migration leaves the source available for diagnosis.
func migrateLegacyConfigTable(db *sql.DB) error {
	var exists int
	if err := db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'app_config'
	)`).Scan(&exists); err != nil {
		return fmt.Errorf("inspect legacy config table: %w", err)
	}
	if exists == 0 {
		return nil
	}

	var settingsCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM gateway_settings").Scan(&settingsCount); err != nil {
		return fmt.Errorf("inspect normalized config: %w", err)
	}
	if settingsCount > 0 {
		if _, err := db.Exec("DROP TABLE IF EXISTS app_config"); err != nil {
			return fmt.Errorf("remove stale legacy config table: %w", err)
		}
		return nil
	}

	var data string
	err := db.QueryRow("SELECT data FROM app_config ORDER BY id LIMIT 1").Scan(&data)
	if err == sql.ErrNoRows || strings.TrimSpace(data) == "" {
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read legacy config: %w", err)
		}
		if _, dropErr := db.Exec("DROP TABLE IF EXISTS app_config"); dropErr != nil {
			return fmt.Errorf("remove empty legacy config table: %w", dropErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy config: %w", err)
	}

	var cfg AppConfig
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		return fmt.Errorf("parse legacy sqlite config: %w", err)
	}
	if err := ValidateConfig(&cfg); err != nil {
		return fmt.Errorf("validate legacy sqlite config: %w", err)
	}
	NormalizeConfig(&cfg)
	if err := ValidateConfig(&cfg); err != nil {
		return fmt.Errorf("validate normalized legacy sqlite config: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin legacy config migration: %w", err)
	}
	if err := writeNormalizedConfig(tx, cfg); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("migrate legacy sqlite config: %w", err)
	}
	if _, err := tx.Exec("DROP TABLE IF EXISTS app_config"); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("remove legacy sqlite config table: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit legacy config migration: %w", err)
	}
	return nil
}

type persistedUpstream struct {
	ID   int64
	Name string
}

func readNormalizedConfig(db *sql.DB) (AppConfig, error) {
	cfg := AppConfig{
		ModelAlias:         map[string]ModelAlias{},
		ReasoningEffortMap: map[string]string{},
		Upstreams:          map[string]*UpstreamConfig{},
	}
	upstreamsByID := make(map[int64]*UpstreamConfig)
	upstreamNamesByID := make(map[int64]string)

	rows, err := db.Query(`SELECT id, name, base_url, proxy_addr, api_type, bridge_mode,
		capabilities_json, responses_reasoning_format, max_retries
		FROM upstreams ORDER BY sort_order, id`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var value persistedUpstream
		var upstream UpstreamConfig
		var apiType, bridgeMode, capabilitiesJSON string
		var maxRetries sql.NullInt64
		if err := rows.Scan(&value.ID, &value.Name, &upstream.BaseURL, &upstream.Proxy, &apiType, &bridgeMode,
			&capabilitiesJSON, &upstream.ResponsesReasoningFormat, &maxRetries); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		upstream.APIType = UpstreamType(apiType)
		upstream.BridgeMode = BridgeMode(bridgeMode)
		if strings.TrimSpace(capabilitiesJSON) != "" {
			var capabilities map[string]bool
			if err := json.Unmarshal([]byte(capabilitiesJSON), &capabilities); err == nil {
				upstream.Capabilities = capabilities
			}
		}
		if maxRetries.Valid {
			retries := int(maxRetries.Int64)
			upstream.MaxRetries = &retries
		}
		upstream.ID = value.ID
		cfg.Upstreams[value.Name] = &upstream
		cfg.UpstreamOrder = append(cfg.UpstreamOrder, value.Name)
		upstreamsByID[value.ID] = cfg.Upstreams[value.Name]
		upstreamNamesByID[value.ID] = value.Name
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	rows, err = db.Query(`SELECT upstream_id, api_key
		FROM upstream_api_keys ORDER BY upstream_id, sort_order, id`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var upstreamID int64
		var apiKey string
		if err := rows.Scan(&upstreamID, &apiKey); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		upstream := upstreamsByID[upstreamID]
		if upstream == nil || strings.TrimSpace(apiKey) == "" {
			continue
		}
		if upstream.APIKey != "" {
			upstream.APIKey += "\n"
		}
		upstream.APIKey += apiKey
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	rows, err = db.Query(`SELECT upstream_id, model_name
		FROM upstream_models ORDER BY upstream_id, sort_order, id`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var upstreamID int64
		var model string
		if err := rows.Scan(&upstreamID, &model); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		if upstream := upstreamsByID[upstreamID]; upstream != nil {
			upstream.CustomModels = append(upstream.CustomModels, model)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	aliasNamesByID := make(map[int64]string)
	rows, err = db.Query(`SELECT id, alias, with_reasoning
		FROM model_aliases ORDER BY sort_order, id`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var id int64
		var aliasName string
		var withReasoning int
		if err := rows.Scan(&id, &aliasName, &withReasoning); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		cfg.ModelAlias[aliasName] = ModelAlias{WithReasoning: withReasoning != 0}
		aliasNamesByID[id] = aliasName
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	rows, err = db.Query(`SELECT alias_id, upstream_id, target_model, weight
		FROM model_alias_targets ORDER BY alias_id, sort_order, id`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var aliasID, upstreamID int64
		var targetModel string
		var weight int
		if err := rows.Scan(&aliasID, &upstreamID, &targetModel, &weight); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		aliasName := aliasNamesByID[aliasID]
		upstreamName := upstreamNamesByID[upstreamID]
		if aliasName == "" || upstreamName == "" {
			continue
		}
		alias := cfg.ModelAlias[aliasName]
		alias.Targets = append(alias.Targets, ModelAliasTarget{
			TargetModel: targetModel,
			Upstream:    upstreamName,
			Weight:      weight,
		})
		cfg.ModelAlias[aliasName] = alias
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	rows, err = db.Query(`SELECT alias_id, request_effort, mapped_effort
		FROM alias_reasoning_effort_mappings ORDER BY alias_id, request_effort`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var aliasID int64
		var requestEffort, mappedEffort string
		if err := rows.Scan(&aliasID, &requestEffort, &mappedEffort); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		aliasName := aliasNamesByID[aliasID]
		if aliasName == "" {
			continue
		}
		alias := cfg.ModelAlias[aliasName]
		if alias.ReasoningEffortMap == nil {
			alias.ReasoningEffortMap = map[string]string{}
		}
		alias.ReasoningEffortMap[requestEffort] = mappedEffort
		cfg.ModelAlias[aliasName] = alias
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	rows, err = db.Query(`SELECT request_effort, mapped_effort
		FROM global_reasoning_effort_mappings ORDER BY request_effort`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var requestEffort, mappedEffort string
		if err := rows.Scan(&requestEffort, &mappedEffort); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		cfg.ReasoningEffortMap[requestEffort] = mappedEffort
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	rows, err = db.Query(`SELECT id, name, secret, disabled, created_at
		FROM api_keys ORDER BY sort_order, id`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var value APIKey
		var disabled int
		if err := rows.Scan(&value.ID, &value.Name, &value.Key, &disabled, &value.CreatedAt); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		value.Disabled = disabled != 0
		cfg.APIKeys = append(cfg.APIKeys, value)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	rows, err = db.Query(`SELECT addr, username, password, name
		FROM socks5_proxies ORDER BY sort_order, id`)
	if err != nil {
		return cfg, err
	}
	for rows.Next() {
		var proxy Socks5Proxy
		if err := rows.Scan(&proxy.Addr, &proxy.Username, &proxy.Password, &proxy.Name); err != nil {
			_ = rows.Close()
			return cfg, err
		}
		cfg.Socks5Proxies = append(cfg.Socks5Proxies, proxy)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return cfg, err
	}
	if err := rows.Close(); err != nil {
		return cfg, err
	}

	var (
		defaultUpstreamID sql.NullInt64
		activeProxyMode   string
		activeProxyID     sql.NullInt64
	)
	err = db.QueryRow(`SELECT default_upstream_id, active_proxy_mode, active_proxy_id
		FROM gateway_settings WHERE id = 1`).Scan(&defaultUpstreamID, &activeProxyMode, &activeProxyID)
	if err != nil && err != sql.ErrNoRows {
		return cfg, err
	}
	if err == nil {
		if defaultUpstreamID.Valid {
			cfg.DefaultUpstream = upstreamNamesByID[defaultUpstreamID.Int64]
			cfg.LegacyDefaultUpstream = cfg.DefaultUpstream != ""
		}
		switch activeProxyMode {
		case configActiveProxyRoundRobin:
			cfg.ActiveSocks5 = socks5RR
		case configActiveProxyRateLimitSwitch:
			cfg.ActiveSocks5 = socks5RateLimitSwitch
		case configActiveProxyRateLimitNoDirect:
			cfg.ActiveSocks5 = socks5RateLimitSwitchNoDirect
		case configActiveProxyFixed:
			if activeProxyID.Valid {
				var addr string
				if lookupErr := db.QueryRow("SELECT addr FROM socks5_proxies WHERE id = ?", activeProxyID.Int64).Scan(&addr); lookupErr == nil {
					cfg.ActiveSocks5 = addr
				}
			}
		}
	}

	var webSearch WebSearchConfig
	var webSearchEnabled int
	err = db.QueryRow(`SELECT enabled, provider, fallback_provider, base_url, api_key,
		searxng_mode, searxng_directory_url, max_results, timeout_seconds,
		max_tool_rounds, max_result_bytes
		FROM web_search_settings WHERE id = 1`).Scan(
		&webSearchEnabled, &webSearch.Provider, &webSearch.FallbackProvider, &webSearch.BaseURL,
		&webSearch.APIKey, &webSearch.SearXNGMode, &webSearch.SearXNGDirectoryURL,
		&webSearch.MaxResults, &webSearch.TimeoutSeconds, &webSearch.MaxToolRounds,
		&webSearch.MaxResultBytes)
	if err != nil && err != sql.ErrNoRows {
		return cfg, err
	}
	if err == nil {
		webSearch.Enabled = webSearchEnabled != 0
	}
	cfg.WebSearch = webSearch
	return cfg, nil
}

func writeNormalizedConfig(tx *sql.Tx, cfg AppConfig) error {
	// Clear children explicitly as well as their parents. This makes the
	// replacement independent of SQLite's FK pragma and guarantees that a
	// removed configuration item cannot remain as an orphan.
	for _, statement := range []string{
		"DELETE FROM gateway_settings",
		"DELETE FROM alias_reasoning_effort_mappings",
		"DELETE FROM model_alias_targets",
		"DELETE FROM model_aliases",
		"DELETE FROM global_reasoning_effort_mappings",
		"DELETE FROM upstream_api_keys",
		"DELETE FROM upstream_models",
		"DELETE FROM api_keys",
		"DELETE FROM web_search_settings",
		"DELETE FROM socks5_proxies",
		"DELETE FROM upstreams",
	} {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}

	upstreamNames := NormalizeUpstreamOrder(cfg.UpstreamOrder, cfg.Upstreams)
	upstreamIDs := make(map[string]int64, len(upstreamNames))
	for index, name := range upstreamNames {
		upstream := cfg.Upstreams[name]
		if upstream == nil {
			continue
		}
		var maxRetries any
		if upstream.MaxRetries != nil {
			maxRetries = *upstream.MaxRetries
		}
		var result sql.Result
		var err error
		capabilitiesJSON := "{}"
		if upstream.Capabilities != nil {
			encoded, err := json.Marshal(upstream.Capabilities)
			if err != nil {
				return err
			}
			capabilitiesJSON = string(encoded)
		}
		if upstream.ID > 0 {
			result, err = tx.Exec(`INSERT INTO upstreams
				(id, name, base_url, proxy_addr, api_type, bridge_mode, capabilities_json, responses_reasoning_format, max_retries, sort_order)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, upstream.ID, name, upstream.BaseURL, upstream.Proxy, upstream.APIType,
				upstream.BridgeMode, capabilitiesJSON, upstream.ResponsesReasoningFormat, maxRetries, index)
		} else {
			result, err = tx.Exec(`INSERT INTO upstreams
				(name, base_url, proxy_addr, api_type, bridge_mode, capabilities_json, responses_reasoning_format, max_retries, sort_order)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, name, upstream.BaseURL, upstream.Proxy, upstream.APIType,
				upstream.BridgeMode, capabilitiesJSON, upstream.ResponsesReasoningFormat, maxRetries, index)
		}
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		upstreamIDs[name] = id

		for keyIndex, apiKey := range splitUpstreamAPIKeys(upstream.APIKey) {
			if _, err := tx.Exec(`INSERT INTO upstream_api_keys(upstream_id, api_key, sort_order)
				VALUES (?, ?, ?)`, id, apiKey, keyIndex); err != nil {
				return err
			}
		}
		for modelIndex, model := range upstream.CustomModels {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO upstream_models(upstream_id, model_name, sort_order)
				VALUES (?, ?, ?)`, id, model, modelIndex); err != nil {
				return err
			}
		}
	}

	aliasNames := make([]string, 0, len(cfg.ModelAlias))
	for name := range cfg.ModelAlias {
		aliasNames = append(aliasNames, name)
	}
	sort.Strings(aliasNames)
	for aliasIndex, aliasName := range aliasNames {
		alias := cfg.ModelAlias[aliasName]
		result, err := tx.Exec(`INSERT INTO model_aliases(alias, with_reasoning, sort_order)
			VALUES (?, ?, ?)`, aliasName, boolToSQLite(alias.WithReasoning), aliasIndex)
		if err != nil {
			return err
		}
		aliasID, err := result.LastInsertId()
		if err != nil {
			return err
		}
		targets := aliasTargetsForStorage(alias)
		for targetIndex, target := range targets {
			upstreamID := upstreamIDs[strings.TrimSpace(target.Upstream)]
			if upstreamID == 0 {
				return fmt.Errorf("alias %q references unknown upstream %q", aliasName, target.Upstream)
			}
			if _, err := tx.Exec(`INSERT INTO model_alias_targets
				(alias_id, upstream_id, target_model, weight, sort_order)
				VALUES (?, ?, ?, ?, ?)`, aliasID, upstreamID, strings.TrimSpace(target.TargetModel), target.Weight, targetIndex); err != nil {
				return err
			}
		}
		for _, mapping := range sortedStringMap(alias.ReasoningEffortMap) {
			requestEffort, mappedEffort := mapping[0], mapping[1]
			if _, err := tx.Exec(`INSERT INTO alias_reasoning_effort_mappings
				(alias_id, request_effort, mapped_effort) VALUES (?, ?, ?)`, aliasID,
				requestEffort, mappedEffort); err != nil {
				return err
			}
		}
	}

	for _, mapping := range sortedStringMap(cfg.ReasoningEffortMap) {
		requestEffort, mappedEffort := mapping[0], mapping[1]
		if _, err := tx.Exec(`INSERT INTO global_reasoning_effort_mappings
			(request_effort, mapped_effort) VALUES (?, ?)`, requestEffort, mappedEffort); err != nil {
			return err
		}
	}

	for index, value := range cfg.APIKeys {
		if _, err := tx.Exec(`INSERT INTO api_keys
			(id, name, secret, disabled, sort_order, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			value.ID, value.Name, value.Key, boolToSQLite(value.Disabled), index, value.CreatedAt); err != nil {
			return err
		}
	}

	proxyIDs := make(map[string]int64, len(cfg.Socks5Proxies))
	for index, proxy := range cfg.Socks5Proxies {
		result, err := tx.Exec(`INSERT INTO socks5_proxies
			(addr, username, password, name, sort_order) VALUES (?, ?, ?, ?, ?)`,
			proxy.Addr, proxy.Username, proxy.Password, proxy.Name, index)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		proxyIDs[proxy.Addr] = id
	}

	webSearch := cfg.WebSearch
	if _, err := tx.Exec(`INSERT INTO web_search_settings
		(id, enabled, provider, fallback_provider, base_url, api_key, searxng_mode,
		searxng_directory_url, max_results, timeout_seconds, max_tool_rounds, max_result_bytes)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, boolToSQLite(webSearch.Enabled),
		webSearch.Provider, webSearch.FallbackProvider, webSearch.BaseURL, webSearch.APIKey,
		webSearch.SearXNGMode, webSearch.SearXNGDirectoryURL, webSearch.MaxResults,
		webSearch.TimeoutSeconds, webSearch.MaxToolRounds, webSearch.MaxResultBytes); err != nil {
		return err
	}

	defaultUpstreamID := any(nil)
	if cfg.LegacyDefaultUpstream {
		if id := upstreamIDs[strings.TrimSpace(cfg.DefaultUpstream)]; id != 0 {
			defaultUpstreamID = id
		}
	}
	activeMode := configActiveProxyDirect
	activeProxyID := any(nil)
	switch strings.TrimSpace(cfg.ActiveSocks5) {
	case socks5RR:
		activeMode = configActiveProxyRoundRobin
	case socks5RateLimitSwitch:
		activeMode = configActiveProxyRateLimitSwitch
	case socks5RateLimitSwitchNoDirect:
		activeMode = configActiveProxyRateLimitNoDirect
	case "":
	default:
		if id := proxyIDs[strings.TrimSpace(cfg.ActiveSocks5)]; id != 0 {
			activeMode = configActiveProxyFixed
			activeProxyID = id
		}
	}
	if _, err := tx.Exec(`INSERT INTO gateway_settings
		(id, default_upstream_id, active_proxy_mode, active_proxy_id) VALUES (1, ?, ?, ?)`,
		defaultUpstreamID, activeMode, activeProxyID); err != nil {
		return err
	}
	return nil
}

func splitUpstreamAPIKeys(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	result := make([]string, 0, strings.Count(raw, "\n")+1)
	seen := make(map[string]struct{})
	for _, value := range strings.Split(raw, "\n") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func aliasTargetsForStorage(alias ModelAlias) []ModelAliasTarget {
	if len(alias.Targets) > 0 {
		return alias.Targets
	}
	// Keep the aggregate API tolerant of the old single-target shape while
	// storing even that shape in the normalized child table.
	if strings.TrimSpace(alias.TargetModel) != "" && strings.TrimSpace(alias.Upstream) != "" {
		return []ModelAliasTarget{{
			TargetModel: strings.TrimSpace(alias.TargetModel),
			Upstream:    strings.TrimSpace(alias.Upstream),
			Weight:      1,
		}}
	}
	return nil
}

func sortedStringMap(values map[string]string) [][2]string {
	keys := make([]string, 0, len(values))
	for key := range values {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([][2]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, [2]string{key, strings.TrimSpace(values[key])})
	}
	return result
}

func boolToSQLite(value bool) int {
	if value {
		return 1
	}
	return 0
}
