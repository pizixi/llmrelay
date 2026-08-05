package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"llmrelay/backend/internal/storage"
)

func loadSQLiteConfig(path string) (AppConfig, error) {
	var cfg AppConfig
	db, err := storage.Open(path)
	if err != nil {
		return cfg, err
	}
	defer db.Close()
	var data string
	err = db.QueryRow("SELECT data FROM app_config WHERE id = 1").Scan(&data)
	if err == sql.ErrNoRows {
		NormalizeConfig(&cfg)
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read sqlite config: %w", err)
	}
	if err := json.Unmarshal([]byte(data), &cfg); err != nil {
		return cfg, fmt.Errorf("parse sqlite config: %w", err)
	}
	if err := ValidateConfig(&cfg); err != nil {
		return cfg, fmt.Errorf("validate config: %w", err)
	}
	NormalizeConfig(&cfg)
	return cfg, nil
}

func saveSQLiteConfig(path string, cfg AppConfig) error {
	NormalizeConfig(&cfg)
	cfg.Upstream = nil
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	db, err := storage.Open(path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO app_config(id, data, updated_at) VALUES(1, ?, CURRENT_TIMESTAMP)
        ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`, string(data)); err != nil {
		return fmt.Errorf("save sqlite config: %w", err)
	}
	return nil
}

// MigrateLegacyJSON imports a config.json only when the SQLite database does
// not already contain a configuration. The source file remains untouched.
func MigrateLegacyJSON(databasePath, legacyPath string) error {
	legacyPath = strings.TrimSpace(legacyPath)
	if legacyPath == "" || !storage.IsSQLitePath(databasePath) {
		return nil
	}
	if _, err := os.Stat(legacyPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	db, err := storage.Open(databasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM app_config").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cfg, err := LoadConfig(legacyPath)
	if err != nil {
		return fmt.Errorf("load legacy config %s: %w", legacyPath, err)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO app_config(id, data, updated_at) VALUES(1, ?, CURRENT_TIMESTAMP)`, string(data))
	if err != nil {
		return fmt.Errorf("import legacy config: %w", err)
	}
	return nil
}
