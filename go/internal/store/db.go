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
