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
//
// Pragmas are passed as DSN options so they apply to EVERY pooled
// connection: foreign keys on, WAL journaling, and a 5s busy timeout so a
// held write lock surfaces as SQLITE_BUSY instead of stalling forever.
func OpenDB(dsn string) (*DB, error) {
	connStr := dsn + "?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"

	sqlDB, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("open sqlite3 %q: %w", dsn, err)
	}

	if dsn == ":memory:" {
		// Each connection to :memory: is a separate database — the pool
		// must stay at one connection or tests see phantom empty DBs.
		sqlDB.SetMaxOpenConns(1)
	} else {
		// A single-connection pool deadlocks the moment any code path
		// holds a result set (or transaction) while acquiring a second
		// connection. WAL supports concurrent readers plus one writer.
		sqlDB.SetMaxOpenConns(4)
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
