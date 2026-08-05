package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenAddsUsageTimingColumnsToExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	_, err = legacy.Exec(`CREATE TABLE usage_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_model TEXT NOT NULL,
		upstream_name TEXT NOT NULL,
		upstream_model TEXT NOT NULL,
		called_at INTEGER NOT NULL,
		called_date TEXT NOT NULL,
		prompt_tokens INTEGER NOT NULL DEFAULT 0,
		completion_tokens INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		request_count INTEGER NOT NULL DEFAULT 1,
		source TEXT NOT NULL DEFAULT 'gateway'
	)`)
	if err != nil {
		legacy.Close()
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`INSERT INTO usage_records
		(request_model, upstream_name, upstream_model, called_at, called_date)
		VALUES ('chat', 'primary', 'gpt-test', 1, '2026-08-05')`)
	if err != nil {
		t.Fatalf("insert migrated record: %v", err)
	}
	var firstByteMS, durationMS int64
	if err := db.QueryRow("SELECT first_byte_ms, duration_ms FROM usage_records LIMIT 1").Scan(&firstByteMS, &durationMS); err != nil {
		t.Fatalf("query migrated timing columns: %v", err)
	}
	if firstByteMS != 0 || durationMS != 0 {
		t.Fatalf("unexpected migrated timing defaults: first=%d duration=%d", firstByteMS, durationMS)
	}
	var apiKeyID, apiKeyName string
	if err := db.QueryRow("SELECT api_key_id, api_key_name FROM usage_records LIMIT 1").Scan(&apiKeyID, &apiKeyName); err != nil {
		t.Fatalf("query migrated api key columns: %v", err)
	}
	if apiKeyID != "" || apiKeyName != "" {
		t.Fatalf("unexpected migrated api key defaults: id=%q name=%q", apiKeyID, apiKeyName)
	}
}
