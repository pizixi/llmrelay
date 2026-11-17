package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenAddsUpstreamKeySlotColumnsToExistingDatabase(t *testing.T) {
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
	_, err = legacy.Exec(`INSERT INTO usage_records (request_model, upstream_name, upstream_model, called_at, called_date)
		VALUES ('chat', 'primary', 'gpt-test', 1, '2026-08-05')`)
	if err != nil {
		legacy.Close()
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	defer db.Close()

	var index, total int
	if err := db.QueryRow("SELECT upstream_key_index, upstream_key_total FROM usage_records LIMIT 1").Scan(&index, &total); err != nil {
		t.Fatalf("query key slot columns: %v", err)
	}
	if index != 0 || total != 0 {
		t.Fatalf("unexpected migrated key slot defaults: index=%d total=%d", index, total)
	}
}
