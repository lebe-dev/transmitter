// Package sqlitedb opens the application's local SQLite database.
//
// Several stores (torrent notes, user preferences) keep their tables in the same
// file, each with its own handle, so the connection setup lives here.
package sqlitedb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, keeps CGO_ENABLED=0 builds working
)

// Open opens the SQLite database at path, creating the file and its directory
// when needed.
//
// WAL keeps the single writer from blocking readers; busy_timeout avoids
// spurious "database is locked" errors on slow storage (e.g. an SD card) and
// when another store writes to the same file.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("ping database %q: %w", path, err)
	}
	return db, nil
}
