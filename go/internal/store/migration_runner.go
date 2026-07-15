package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
)

const migrationsDir = "migrations"

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationRunner handles embedded goose migrations.
type MigrationRunner struct {
	log *slog.Logger
	db  *sql.DB
}

// NewMigrationRunner creates a new MigrationRunner. If log is nil, slog.Default
// is used so internal guards aren't needed.
func NewMigrationRunner(log *slog.Logger, db *sql.DB) *MigrationRunner {
	if log == nil {
		log = slog.Default()
	}
	return &MigrationRunner{log: log, db: db}
}

// Run applies all pending migrations in version order.
// It is idempotent: already-applied migrations are skipped.
func (mr *MigrationRunner) Run(ctx context.Context) error {
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())

	if err := mr.seedGooseFromLegacyMigrations(ctx); err != nil {
		return fmt.Errorf("seed goose migration versions: %w", err)
	}

	mr.log.Debug("running embedded goose migrations")
	if err := goose.UpContext(ctx, mr.db, migrationsDir); err != nil {
		return fmt.Errorf("run goose migrations: %w", err)
	}
	return nil
}

func (mr *MigrationRunner) seedGooseFromLegacyMigrations(ctx context.Context) error {
	hasLegacyTable, err := tableExists(ctx, mr.db, "schema_migrations")
	if err != nil {
		return fmt.Errorf("check schema_migrations table: %w", err)
	}
	if !hasLegacyTable {
		return nil
	}

	hasGooseTable, err := tableExists(ctx, mr.db, goose.TableName())
	if err != nil {
		return fmt.Errorf("check %s table: %w", goose.TableName(), err)
	}
	if hasGooseTable {
		return nil
	}

	legacyVersions, err := legacyMigrationVersions(ctx, mr.db)
	if err != nil {
		return fmt.Errorf("read legacy migration versions: %w", err)
	}
	if len(legacyVersions) == 0 {
		return nil
	}

	if _, err := goose.EnsureDBVersionContext(ctx, mr.db); err != nil {
		return fmt.Errorf("create %s table: %w", goose.TableName(), err)
	}

	const insertVersion = `INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, ?)`
	for _, version := range legacyVersions {
		if version == 0 {
			continue
		}
		if _, err := mr.db.ExecContext(ctx, insertVersion, version, true); err != nil {
			return fmt.Errorf("insert goose version %d: %w", version, err)
		}
	}
	mr.log.Info("seeded goose migration versions from legacy schema_migrations", "count", len(legacyVersions))
	return nil
}

func tableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	const query = `SELECT 1 FROM sqlite_master WHERE type='table' AND name=? LIMIT 1`
	var found int
	if err := db.QueryRowContext(ctx, query, tableName).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func legacyMigrationVersions(ctx context.Context, db *sql.DB) ([]int64, error) {
	const query = `SELECT version FROM schema_migrations ORDER BY version ASC`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := []int64{}
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}
