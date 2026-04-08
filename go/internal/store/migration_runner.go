package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationRunner handles versioned, filesystem-based migrations.
type MigrationRunner struct {
	log *slog.Logger
	db  *sql.DB
}

// NewMigrationRunner creates a new MigrationRunner.
func NewMigrationRunner(log *slog.Logger, db *sql.DB) *MigrationRunner {
	return &MigrationRunner{log: log, db: db}
}

// Run applies all pending migrations in version order.
// It is idempotent: already-applied migrations are skipped.
func (mr *MigrationRunner) Run(ctx context.Context) error {
	if err := mr.createMigrationTable(ctx); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	applied, err := mr.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("get applied migrations: %w", err)
	}

	migrations, err := mr.loadMigrationFiles()
	if err != nil {
		return fmt.Errorf("load migration files: %w", err)
	}

	for _, mig := range migrations {
		if applied[mig.Version] {
			if mr.log != nil {
				mr.log.Info("migration already applied", "version", mig.Version, "description", mig.Description)
			}
			continue
		}

		if err := mr.applyMigration(ctx, mig); err != nil {
			return fmt.Errorf("apply migration %s: %w", mig.Name, err)
		}

		if mr.log != nil {
			mr.log.Info("migration applied", "version", mig.Version, "description", mig.Description)
		}
	}

	return nil
}

// Migration represents a single versioned SQL migration file.
type Migration struct {
	Version     int
	Name        string
	Description string
	Content     string
}

// loadMigrationFiles reads all .sql files from the embedded assets directory,
// parses their version number, and returns them sorted by version ascending.
func (mr *MigrationRunner) loadMigrationFiles() ([]Migration, error) {
	const dir = "migrations"

	entries, err := migrationsFS.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	migrations := make([]Migration, 0, len(entries))

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		content, err := migrationsFS.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read migration file %s: %w", name, err)
		}

		version := parseVersion(name)
		description := extractDescription(string(content))

		migrations = append(migrations, Migration{
			Version:     version,
			Name:        name,
			Description: description,
			Content:     string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// parseVersion extracts the leading integer from a filename like "001_init.sql".
func parseVersion(name string) int {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) == 0 {
		return 0
	}
	var v int
	fmt.Sscanf(parts[0], "%d", &v)
	return v
}

// extractDescription finds the "-- Description:" comment line in SQL content.
func extractDescription(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "Description:") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-- Description:"))
		}
	}
	return ""
}

// createMigrationTable ensures the schema_migrations tracking table exists.
func (mr *MigrationRunner) createMigrationTable(ctx context.Context) error {
	const query = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     INTEGER PRIMARY KEY,
			description TEXT    NOT NULL,
			applied_at  DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := mr.db.ExecContext(ctx, query)
	return err
}

// getAppliedMigrations returns a set of already-applied version numbers.
func (mr *MigrationRunner) getAppliedMigrations(ctx context.Context) (map[int]bool, error) {
	rows, err := mr.db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}

	return applied, rows.Err()
}

// applyMigration executes a single migration SQL and records it atomically.
func (mr *MigrationRunner) applyMigration(ctx context.Context, mig Migration) error {
	tx, err := mr.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, mig.Content); err != nil {
		return err
	}

	const record = `INSERT INTO schema_migrations (version, description) VALUES (?, ?)`
	if _, err := tx.ExecContext(ctx, record, mig.Version, mig.Description); err != nil {
		return err
	}

	return tx.Commit()
}
