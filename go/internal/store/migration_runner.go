package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strconv"

	"github.com/pressly/goose/v3"
)

const migrationsDir = "migrations"

// reconcileBaselineVersion is the highest migration version whose effects
// reconciledSchemaVersion verifies. Schema-signature reconciliation must never
// mark versions past it as applied: a DB that lost its goose metadata may
// predate newer migrations, and marking them applied would silently skip
// them. Bump this only together with new checks in reconciledSchemaVersion.
const (
	reconcileBaselineVersion  int64 = 34
	preHydrationSchemaVersion int64 = 31
	preMentionedSchemaVersion int64 = 32
	preReadStateSchemaVersion int64 = 33
)

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
	migrationFS, err := fs.Sub(migrationsFS, migrationsDir)
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}

	if err := mr.seedGooseFromLegacyMigrations(ctx); err != nil {
		return fmt.Errorf("seed goose migration versions: %w", err)
	}
	if err := mr.reconcileGooseFromCurrentSchema(ctx, reconcileBaselineVersion); err != nil {
		return fmt.Errorf("reconcile goose migration versions: %w", err)
	}
	if err := repairMalformedReadStateSchema(ctx, mr.db); err != nil {
		return fmt.Errorf("repair pr_read_state schema: %w", err)
	}

	mr.log.Debug("running embedded goose migrations")
	if err := mr.runGooseProvider(ctx, migrationFS); err != nil {
		return err
	}
	return nil
}

func (mr *MigrationRunner) runGooseProvider(ctx context.Context, migrationFS fs.FS) error {
	if err := mr.ensureGooseVersionTable(ctx); err != nil {
		return fmt.Errorf("create %s table: %w", goose.DefaultTablename, err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, mr.db, migrationFS, goose.WithLogger(goose.NopLogger()))
	if err != nil {
		return fmt.Errorf("create goose provider: %w", err)
	}

	if _, err := provider.Up(ctx); err != nil {
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

	hasGooseTable, err := tableExists(ctx, mr.db, goose.DefaultTablename)
	if err != nil {
		return fmt.Errorf("check %s table: %w", goose.DefaultTablename, err)
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

	if err := mr.ensureGooseVersionTable(ctx); err != nil {
		return fmt.Errorf("create %s table: %w", goose.DefaultTablename, err)
	}

	for _, version := range legacyVersions {
		if version == 0 {
			continue
		}
		if err := mr.markGooseVersionApplied(ctx, version); err != nil {
			return fmt.Errorf("insert goose version %d: %w", version, err)
		}
	}
	mr.log.Info("seeded goose migration versions from legacy schema_migrations", "count", len(legacyVersions))
	return nil
}

func (mr *MigrationRunner) reconcileGooseFromCurrentSchema(ctx context.Context, baselineVersion int64) error {
	if err := mr.ensureGooseVersionTable(ctx); err != nil {
		return err
	}

	reconciledVersion, err := reconciledSchemaVersion(ctx, mr.db, baselineVersion)
	if err != nil {
		return fmt.Errorf("check current schema: %w", err)
	}
	for _, version := range schemaMigrationVersions() {
		if version > reconciledVersion {
			continue
		}
		if err := mr.markGooseVersionApplied(ctx, version); err != nil {
			return fmt.Errorf("mark existing schema migration %d applied: %w", version, err)
		}
	}

	return nil
}

func reconciledSchemaVersion(ctx context.Context, db *sql.DB, baselineVersion int64) (int64, error) {
	hasPreHydrationSchema, err := preHydrationSchemaExists(ctx, db)
	if err != nil {
		return 0, err
	}
	if !hasPreHydrationSchema {
		return 0, nil
	}

	hasHydratedColumn, err := columnExists(ctx, db, "prs", "hydrated")
	if err != nil {
		return 0, fmt.Errorf("check prs.hydrated column: %w", err)
	}
	if !hasHydratedColumn {
		return min(baselineVersion, preHydrationSchemaVersion), nil
	}

	hasMentionedColumn, err := columnExists(ctx, db, "prs", "mentioned")
	if err != nil {
		return 0, fmt.Errorf("check prs.mentioned column: %w", err)
	}
	if !hasMentionedColumn {
		return min(baselineVersion, preMentionedSchemaVersion), nil
	}

	hasReadStateSchema, err := readStateSchemaExists(ctx, db)
	if err != nil {
		return 0, err
	}
	if !hasReadStateSchema {
		return min(baselineVersion, preReadStateSchemaVersion), nil
	}

	return baselineVersion, nil
}

// repairMalformedReadStateSchema removes an incomplete pre-existing table so
// migration 34 can recreate it with the required composite primary key. Rows
// are preserved while rebuilding the table.
func repairMalformedReadStateSchema(ctx context.Context, db *sql.DB) error {
	exists, err := tableExists(ctx, db, "pr_read_state")
	if err != nil || !exists {
		return err
	}
	valid, err := readStateSchemaExists(ctx, db)
	if err != nil || valid {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin read-state repair: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	statements := []string{
		`ALTER TABLE pr_read_state RENAME TO pr_read_state_malformed`,
		`CREATE TABLE pr_read_state (
			pr_id TEXT NOT NULL,
			login TEXT NOT NULL,
			last_read_at TIMESTAMP NOT NULL,
			PRIMARY KEY (pr_id, login)
		)`,
		`INSERT OR REPLACE INTO pr_read_state (pr_id, login, last_read_at)
		 SELECT pr_id, login, MAX(last_read_at)
		 FROM pr_read_state_malformed
		 GROUP BY pr_id, login`,
		`DROP TABLE pr_read_state_malformed`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild read-state table: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit read-state repair: %w", err)
	}
	return nil
}

// readStateSchemaExists reports whether pr_read_state matches migration 34:
// the three expected columns plus a PRIMARY KEY or UNIQUE constraint covering
// (pr_id, login). markPRsRead upserts with ON CONFLICT(pr_id, login), which
// SQLite rejects unless such a constraint exists, so reconciling migration 34
// on a table without it would leave read-state writes permanently broken.
func readStateSchemaExists(ctx context.Context, db *sql.DB) (bool, error) {
	for _, columnName := range []string{"pr_id", "login", "last_read_at"} {
		hasColumn, err := columnExists(ctx, db, "pr_read_state", columnName)
		if err != nil {
			return false, fmt.Errorf("check pr_read_state.%s column: %w", columnName, err)
		}
		if !hasColumn {
			return false, nil
		}
	}

	return readStateConflictTargetExists(ctx, db)
}

// readStateConflictTargetExists reports whether (pr_id, login) is covered by the
// pr_read_state PRIMARY KEY or by a UNIQUE index, in either column order.
func readStateConflictTargetExists(ctx context.Context, db *sql.DB) (bool, error) {
	primaryKey, err := primaryKeyColumns(ctx, db, "pr_read_state")
	if err != nil {
		return false, err
	}
	if isReadStateConflictTarget(primaryKey) {
		return true, nil
	}

	uniqueIndexes, err := uniqueIndexColumns(ctx, db, "pr_read_state")
	if err != nil {
		return false, err
	}
	for _, indexColumns := range uniqueIndexes {
		if isReadStateConflictTarget(indexColumns) {
			return true, nil
		}
	}
	return false, nil
}

func isReadStateConflictTarget(columns []string) bool {
	if len(columns) != 2 {
		return false
	}
	return (columns[0] == "pr_id" && columns[1] == "login") ||
		(columns[0] == "login" && columns[1] == "pr_id")
}

// primaryKeyColumns returns the table's primary-key columns in key order.
func primaryKeyColumns(ctx context.Context, db *sql.DB, tableName string) ([]string, error) {
	const query = `SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk`
	rows, err := db.QueryContext(ctx, query, tableName)
	if err != nil {
		return nil, fmt.Errorf("read %s primary key: %w", tableName, err)
	}
	defer rows.Close()

	columns := []string{}
	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err != nil {
			return nil, fmt.Errorf("scan %s primary key: %w", tableName, err)
		}
		columns = append(columns, columnName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read %s primary key: %w", tableName, err)
	}
	return columns, nil
}

// uniqueIndexColumns returns the columns of every UNIQUE index on the table,
// each in index order.
func uniqueIndexColumns(ctx context.Context, db *sql.DB, tableName string) ([][]string, error) {
	const indexQuery = `SELECT name FROM pragma_index_list(?) WHERE "unique" = 1`
	rows, err := db.QueryContext(ctx, indexQuery, tableName)
	if err != nil {
		return nil, fmt.Errorf("list %s indexes: %w", tableName, err)
	}
	defer rows.Close()

	indexNames := []string{}
	for rows.Next() {
		var indexName string
		if err := rows.Scan(&indexName); err != nil {
			return nil, fmt.Errorf("scan %s indexes: %w", tableName, err)
		}
		indexNames = append(indexNames, indexName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list %s indexes: %w", tableName, err)
	}

	indexes := make([][]string, 0, len(indexNames))
	for _, indexName := range indexNames {
		columns, err := indexColumns(ctx, db, indexName)
		if err != nil {
			return nil, err
		}
		indexes = append(indexes, columns)
	}
	return indexes, nil
}

func indexColumns(ctx context.Context, db *sql.DB, indexName string) ([]string, error) {
	const query = `SELECT name FROM pragma_index_info(?) ORDER BY seqno`
	rows, err := db.QueryContext(ctx, query, indexName)
	if err != nil {
		return nil, fmt.Errorf("read index %s: %w", indexName, err)
	}
	defer rows.Close()

	columns := []string{}
	for rows.Next() {
		var columnName sql.NullString
		if err := rows.Scan(&columnName); err != nil {
			return nil, fmt.Errorf("scan index %s: %w", indexName, err)
		}
		if !columnName.Valid {
			// Expression index column; cannot serve as a named conflict target.
			return nil, nil
		}
		columns = append(columns, columnName.String)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read index %s: %w", indexName, err)
	}
	return columns, nil
}

func schemaMigrationVersions() []int64 {
	migrationFiles, err := fs.Glob(migrationsFS, migrationsDir+"/[0-9][0-9][0-9][0-9][0-9]_*.sql")
	if err != nil {
		return []int64{}
	}

	versions := make([]int64, 0, len(migrationFiles))
	for _, migrationFile := range migrationFiles {
		fileName := filepath.Base(migrationFile)
		versionText := fileName[:5]
		version, err := strconv.ParseInt(versionText, 10, 64)
		if err != nil || version == 0 {
			continue
		}
		versions = append(versions, version)
	}
	return versions
}

func (mr *MigrationRunner) ensureGooseVersionTable(ctx context.Context) error {
	createTable := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		version_id INTEGER NOT NULL,
		is_applied INTEGER NOT NULL,
		tstamp TIMESTAMP DEFAULT (datetime('now'))
	)`, goose.DefaultTablename)
	if _, err := mr.db.ExecContext(ctx, createTable); err != nil {
		return err
	}

	insertZeroVersion := fmt.Sprintf(`INSERT INTO %s (version_id, is_applied)
		SELECT 0, 1
		WHERE NOT EXISTS (
			SELECT 1 FROM %s WHERE version_id = 0 AND is_applied = 1
		)`, goose.DefaultTablename, goose.DefaultTablename)
	_, err := mr.db.ExecContext(ctx, insertZeroVersion)
	return err
}

func (mr *MigrationRunner) markGooseVersionApplied(ctx context.Context, version int64) error {
	if err := mr.ensureGooseVersionTable(ctx); err != nil {
		return err
	}

	insertVersion := fmt.Sprintf(`INSERT INTO %s (version_id, is_applied)
		SELECT ?, 1
		WHERE NOT EXISTS (
			SELECT 1 FROM %s WHERE version_id = ? AND is_applied = 1
		)`, goose.DefaultTablename, goose.DefaultTablename)
	_, err := mr.db.ExecContext(ctx, insertVersion, version, version)
	return err
}

func currentSchemaExists(ctx context.Context, db *sql.DB) (bool, error) {
	hasPreHydrationSchema, err := preHydrationSchemaExists(ctx, db)
	if err != nil || !hasPreHydrationSchema {
		return hasPreHydrationSchema, err
	}

	return columnExists(ctx, db, "prs", "hydrated")
}

func preHydrationSchemaExists(ctx context.Context, db *sql.DB) (bool, error) {
	checks := []struct {
		table  string
		column string
	}{
		{table: "prs", column: "repo_id"},
		{table: "pr_diffs", column: "diff_status"},
		{table: "repos", column: "local_path"},
		{table: "comments", column: "author_type"},
		{table: "comments", column: "dedup_group_id"},
	}

	for _, check := range checks {
		hasColumn, err := columnExists(ctx, db, check.table, check.column)
		if err != nil {
			return false, fmt.Errorf("check %s.%s column: %w", check.table, check.column, err)
		}
		if !hasColumn {
			return false, nil
		}
	}

	if hasClassificationState, err := tableExists(ctx, db, "comment_classification_state"); err != nil {
		return false, err
	} else if !hasClassificationState {
		return false, nil
	}

	return tableExists(ctx, db, "file_expertise")
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

func columnExists(ctx context.Context, db *sql.DB, tableName string, columnName string) (bool, error) {
	const query = `SELECT 1 FROM pragma_table_info(?) WHERE name=? LIMIT 1`
	var found int
	if err := db.QueryRowContext(ctx, query, tableName, columnName).Scan(&found); err != nil {
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
