// Package storage owns the small SQLite database shared by configuration and
// usage accounting. The modernc.org driver is pure Go, so the gateway does
// not require CGO on any supported platform.
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS app_config (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    data TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS usage_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_model TEXT NOT NULL,
    upstream_name TEXT NOT NULL,
    upstream_model TEXT NOT NULL,
    api_key_id TEXT NOT NULL DEFAULT '',
    api_key_name TEXT NOT NULL DEFAULT '',
    called_at INTEGER NOT NULL,
    called_date TEXT NOT NULL,
    prompt_tokens INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens INTEGER NOT NULL DEFAULT 0,
    request_count INTEGER NOT NULL DEFAULT 1,
	first_byte_ms INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'gateway'
);
CREATE INDEX IF NOT EXISTS idx_usage_called_at ON usage_records(called_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_usage_called_date ON usage_records(called_date);
CREATE INDEX IF NOT EXISTS idx_usage_model ON usage_records(request_model);
CREATE INDEX IF NOT EXISTS idx_usage_upstream ON usage_records(upstream_name);
`

// IsSQLitePath treats JSON paths as the legacy file format. This keeps the
// package-level helpers source-compatible with older tests and deployments.
func IsSQLitePath(path string) bool {
	return !strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".json")
}

// Open opens and initializes a SQLite database. A single connection avoids
// lock contention for the small embedded workload while WAL keeps readers
// responsive during a write.
func Open(path string) (*sql.DB, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite database path is empty")
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if dir := filepath.Dir(path); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize sqlite pragmas: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize sqlite schema: %w", err)
	}
	if err := ensureUsageTimingColumns(db); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_usage_api_key_name ON usage_records(api_key_name)"); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize api key usage index: %w", err)
	}
	return db, nil
}

func ensureUsageTimingColumns(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(usage_records)")
	if err != nil {
		return fmt.Errorf("inspect usage schema: %w", err)
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("inspect usage column: %w", err)
		}
		columns[name] = true
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("inspect usage schema: %w", err)
	}
	definitions := []string{
		"api_key_id TEXT NOT NULL DEFAULT ''",
		"api_key_name TEXT NOT NULL DEFAULT ''",
		"first_byte_ms INTEGER NOT NULL DEFAULT 0",
		"duration_ms INTEGER NOT NULL DEFAULT 0",
	}
	for _, definition := range definitions {
		column := strings.Fields(definition)[0]
		if columns[column] {
			continue
		}
		if _, err := db.Exec("ALTER TABLE usage_records ADD COLUMN " + definition); err != nil {
			return fmt.Errorf("add usage timing column %s: %w", column, err)
		}
	}
	return nil
}
