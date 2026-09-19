package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/maltekiefer/verbrauch/internal/store"
)

func TestOpen_CreatesSchemaAndPragmas(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sub", "verbrauch.db")

	db, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = db.Close() }()

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 1 {
		t.Errorf("user_version = %d, want 1", version)
	}

	tables := []string{"consumption_types", "readings", "refills"}
	for _, table := range tables {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestOpen_ReopenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "verbrauch.db")

	db1, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first handle: %v", err)
	}

	db2, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer func() { _ = db2.Close() }()

	var version int
	if err := db2.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != 1 {
		t.Errorf("user_version after reopen = %d, want 1", version)
	}
}
