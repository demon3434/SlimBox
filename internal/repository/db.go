package repository

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	SQL *sql.DB
}

// NewDB initializes the SQLite database at dbPath and creates all necessary tables.
func NewDB(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// SQLite performance optimizations for embedded low-power ARM environments
	db.SetMaxOpenConns(1) // Single writer avoids busy locks
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return nil, fmt.Errorf("failed to execute pragma %s: %w", p, err)
		}
	}

	repo := &DB{SQL: db}
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return repo, nil
}

func (d *DB) Close() error {
	return d.SQL.Close()
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		source_file_name TEXT NOT NULL,
		source_file_path TEXT NOT NULL,
		source_file_size INTEGER NOT NULL,
		output_file_name TEXT NOT NULL DEFAULT '',
		output_file_path TEXT NOT NULL DEFAULT '',
		output_file_size INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL,
		media_info_json TEXT NOT NULL DEFAULT '',
		params_json TEXT NOT NULL DEFAULT '',
		progress_json TEXT NOT NULL DEFAULT '',
		error_msg TEXT NOT NULL DEFAULT '',
		download_count INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at TIMESTAMP,
		completed_at TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at DESC);

	CREATE TABLE IF NOT EXISTS custom_profiles (
		name TEXT PRIMARY KEY,
		params_json TEXT NOT NULL,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	_, err := d.SQL.Exec(schema)
	return err
}
