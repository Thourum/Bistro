package store

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps sql.DB to provide a typed handle for the SQLite store.
type DB struct {
	*sql.DB
}

// OpenDB opens (or creates) a SQLite database at the given DSN.
// Use ":memory:" for an in-process ephemeral database.
func OpenDB(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite3 %q: %w", dsn, err)
	}

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite3 %q: %w", dsn, err)
	}

	// SQLite pragmas are per-connection. Pin the pool to one connection so
	// pragmas set below apply to every subsequent query.
	sqlDB.SetMaxOpenConns(1)

	// Enable foreign key enforcement (disabled by default per SQLite connection).
	if _, err := sqlDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable foreign_keys pragma: %w", err)
	}

	// Enable WAL mode for better concurrent read/write performance.
	if _, err := sqlDB.Exec("PRAGMA journal_mode = WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set journal_mode WAL pragma: %w", err)
	}

	// Bound lock waits: if another connection holds the write lock, fail
	// after 5s with SQLITE_BUSY instead of stalling a sync pass forever.
	if _, err := sqlDB.Exec("PRAGMA busy_timeout = 5000"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set busy_timeout pragma: %w", err)
	}

	return &DB{sqlDB}, nil
}

// Ping verifies the database connection is alive.
func (db *DB) Ping() error {
	return db.DB.Ping()
}

// Close releases the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}
