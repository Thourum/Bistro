# Migration Refactoring Plan — From Monolithic to Stacked Migrations

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace single embedded migration string with versioned, filesystem-based stacked migrations for better maintainability and future extensibility.

**Architecture:** Migrations are stored as individual `.sql` files in `go/assets/migrations/` directory, versioned sequentially (001_init.sql, 002_add_feature.sql, etc.). A migration runner tracks applied versions in SQLite and executes pending migrations in order.

**Tech Stack:** Go 1.21+, SQLite, Go's `embed` package for asset bundling (optional).

---

## Current State

**Problem:** `internal/store/migrations.go` contains entire schema as a single `migrationsSQL` string constant (300+ LOC).

**Issues:**
1. ❌ All-or-nothing: Every schema change requires editing the monolithic string
2. ❌ Version tracking: No way to know which schema version is deployed
3. ❌ Incremental updates: Can't apply small changes to existing databases
4. ❌ Not extensible: Hard to maintain as codebase grows
5. ❌ Poor git history: Schema changes are buried in large functions

---

## Target State

```
go/
├── assets/
│   └── migrations/
│       ├── 001_init.sql              ← Initial schema (prs, comments, files tables)
│       ├── 002_add_pr_indexes.sql    ← Add performance indexes
│       └── 003_future_feature.sql    ← Template for Phase 5+
│
└── internal/store/
    ├── migrations.go                 ← Migration loader + runner (120 LOC)
    ├── migrations_test.go            ← New tests for migration versioning
    └── db.go                          ← Updated to use migration runner
```

**Benefits:**
- ✅ Each migration is independent and reviewable
- ✅ Version tracking with `schema_migrations` table
- ✅ Idempotent: Re-running doesn't break existing schemas
- ✅ Incremental: New features add new migrations, not modify old ones
- ✅ Clear git history: Each migration is a separate commit
- ✅ Easy to test: Run migrations in sequence

---

## Migration Design

### File Format: `NNN_feature_name.sql`

```
001_init.sql                  ← Init (creates all tables)
002_add_pr_indexes.sql        ← Enhancement (adds indexes)
003_add_feature_columns.sql   ← Feature (adds new columns)
```

Each file is a **complete, idempotent migration**:
- Uses `CREATE TABLE IF NOT EXISTS` (safe to re-run)
- Uses `CREATE INDEX IF NOT EXISTS` (safe to re-run)
- Includes `-- Migration: 001_init` header comment
- One logical change per file

### Version Tracking Table

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Track applied migrations
INSERT INTO schema_migrations (version, description) VALUES
    (1, 'Initial schema: prs, comments, files tables'),
    (2, 'Add performance indexes');
```

### Migration Execution Strategy

1. Load all migration files from `go/assets/migrations/`
2. Query `schema_migrations` table to find latest applied version
3. Execute all pending migrations in numerical order
4. Update `schema_migrations` table with new versions

---

## File Structure

### Task 1: Create Migration Files

**Files:**
- Create: `go/assets/migrations/001_init.sql`
- Create: `go/assets/migrations/002_add_pr_indexes.sql`
- Create: `go/assets/migrations/003_future_template.sql` (placeholder)

**Steps:**

- [ ] **Step 1: Extract initial schema from current migrations.go**

Current monolithic migration has:
- `prs` table with 19 columns
- `comments` table with 6 columns
- `files` table with 6 columns
- Various indexes

Create `001_init.sql`:
```sql
-- Migration: 001_init
-- Description: Initial schema - create prs, comments, files tables

CREATE TABLE IF NOT EXISTS prs (
    id TEXT PRIMARY KEY,
    pr_id TEXT NOT NULL,
    title TEXT NOT NULL,
    state TEXT NOT NULL,
    owner TEXT NOT NULL,
    repo TEXT NOT NULL,
    number INTEGER NOT NULL,
    body TEXT,
    author TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    merged_at DATETIME,
    UNIQUE(owner, repo, number)
);

CREATE TABLE IF NOT EXISTS comments (
    id TEXT PRIMARY KEY,
    pr_id TEXT NOT NULL,
    body TEXT NOT NULL,
    author TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    line INTEGER DEFAULT 0,
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY,
    pr_id TEXT NOT NULL,
    path TEXT NOT NULL,
    status TEXT NOT NULL,
    patch TEXT,
    added_lines INTEGER DEFAULT 0,
    removed_lines INTEGER DEFAULT 0,
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);
```

- [ ] **Step 2: Create performance indexes migration**

Create `002_add_pr_indexes.sql`:
```sql
-- Migration: 002_add_pr_indexes
-- Description: Add indexes for common queries

CREATE INDEX IF NOT EXISTS idx_prs_state ON prs(state);
CREATE INDEX IF NOT EXISTS idx_prs_owner_repo ON prs(owner, repo);
CREATE INDEX IF NOT EXISTS idx_comments_pr_id ON comments(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_pr_id ON files(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
```

- [ ] **Step 3: Create schema_migrations table**

Create `000_schema_migrations.sql` (runs first):
```sql
-- Migration: 000_schema_migrations
-- Description: Create migration tracking table

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    description TEXT NOT NULL,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 4: Create placeholder for future migrations**

Create `003_future_placeholder.sql`:
```sql
-- Migration: 003_future_placeholder
-- Description: Placeholder for future features (Phase 5+)
-- This will be replaced with actual migrations

-- Example structure for adding a new column:
-- ALTER TABLE prs ADD COLUMN last_synced_at DATETIME DEFAULT NULL;
```

---

### Task 2: Implement Migration Runner

**Files:**
- Modify: `go/internal/store/migrations.go` (replace monolithic string)
- Create: `go/internal/store/migration_runner.go` (new executor)
- Modify: `go/internal/store/db.go` (use new runner)
- Create: `go/internal/store/migrations_test.go` (tests)

**Steps:**

- [ ] **Step 1: Create migration runner**

```go
// go/internal/store/migration_runner.go
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

//go:embed assets/migrations/*.sql
var migrationsFS embed.FS

// MigrationRunner handles versioned migrations
type MigrationRunner struct {
	log *slog.Logger
	db  *sql.DB
}

// NewMigrationRunner creates a new migration runner
func NewMigrationRunner(log *slog.Logger, db *sql.DB) *MigrationRunner {
	return &MigrationRunner{
		log: log,
		db:  db,
	}
}

// Run applies all pending migrations in order
func (mr *MigrationRunner) Run(ctx context.Context) error {
	// Create schema_migrations table if not exists
	if err := mr.createMigrationTable(ctx); err != nil {
		return fmt.Errorf("failed to create migration table: %w", err)
	}

	// Get applied migrations
	applied, err := mr.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Load migration files
	migrations, err := mr.loadMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to load migration files: %w", err)
	}

	// Execute pending migrations
	for _, mig := range migrations {
		if _, found := applied[mig.Version]; found {
			if mr.log != nil {
				mr.log.Info("migration already applied", "version", mig.Version, "description", mig.Description)
			}
			continue
		}

		if err := mr.applyMigration(ctx, mig); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", mig.Name, err)
		}

		if mr.log != nil {
			mr.log.Info("migration applied", "version", mig.Version, "description", mig.Description)
		}
	}

	return nil
}

// Migration represents a single migration file
type Migration struct {
	Version     int    // e.g., 1, 2, 3
	Name        string // e.g., "001_init.sql"
	Description string // Extracted from comment
	Content     string // SQL content
}

// loadMigrationFiles loads all migration files from assets
func (mr *MigrationRunner) loadMigrationFiles() ([]Migration, error) {
	entries, err := migrationsFS.ReadDir("assets/migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations dir: %w", err)
	}

	var migrations []Migration

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		content, err := migrationsFS.ReadFile(filepath.Join("assets/migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}

		// Parse version from filename (e.g., "001_init.sql" -> 1)
		parts := strings.Split(entry.Name(), "_")
		version := 0
		fmt.Sscanf(parts[0], "%d", &version)

		// Extract description from first comment
		description := extractDescription(string(content))

		migrations = append(migrations, Migration{
			Version:     version,
			Name:        entry.Name(),
			Description: description,
			Content:     string(content),
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// extractDescription extracts description from SQL comment
func extractDescription(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Description:") {
			return strings.TrimPrefix(strings.TrimSpace(line), "-- Description:")
		}
	}
	return ""
}

// createMigrationTable creates the schema_migrations table
func (mr *MigrationRunner) createMigrationTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			description TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := mr.db.ExecContext(ctx, query)
	return err
}

// getAppliedMigrations returns a map of applied migration versions
func (mr *MigrationRunner) getAppliedMigrations(ctx context.Context) (map[int]bool, error) {
	rows, err := mr.db.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

// applyMigration executes a single migration and records it
func (mr *MigrationRunner) applyMigration(ctx context.Context, mig Migration) error {
	tx, err := mr.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.ExecContext(ctx, mig.Content); err != nil {
		return err
	}

	// Record migration
	query := `INSERT INTO schema_migrations (version, description) VALUES (?, ?)`
	if _, err := tx.ExecContext(ctx, query, mig.Version, mig.Description); err != nil {
		return err
	}

	return tx.Commit()
}
```

- [ ] **Step 2: Update db.go to use migration runner**

In `db.go`, replace the old migrations code:
```go
// OLD:
// if err := runMigrations(db); err != nil { ... }

// NEW:
if err := NewMigrationRunner(log, db).Run(ctx); err != nil {
    return nil, fmt.Errorf("failed to run migrations: %w", err)
}
```

- [ ] **Step 3: Remove old monolithic migrations.go**

Delete the old `migrationsSQL` constant and related functions. Keep only the new MigrationRunner.

- [ ] **Step 4: Write tests**

```go
// go/internal/store/migrations_test.go
package store

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
)

func TestMigrationRunnerLoadsFiles(t *testing.T) {
	runner := NewMigrationRunner(nil, nil)
	migrations, err := runner.loadMigrationFiles()

	if err != nil {
		t.Fatalf("loadMigrationFiles error = %v", err)
	}
	if len(migrations) == 0 {
		t.Error("expected at least one migration file")
	}

	// Verify migrations are sorted by version
	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version <= migrations[i-1].Version {
			t.Errorf("migrations not sorted by version")
		}
	}
}

func TestMigrationRunnerExecutesInOrder(t *testing.T) {
	// Integration test with temporary database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	log := slog.New(slog.NewTextHandler(nil, nil))
	runner := NewMigrationRunner(log, db)

	ctx := context.Background()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify tables exist
	tables := []string{"prs", "comments", "files", "schema_migrations"}
	for _, table := range tables {
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		if err != nil {
			t.Errorf("failed to verify table %s exists: %v", table, err)
		}
	}

	// Verify migrations were recorded
	var migCount int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&migCount)
	if migCount == 0 {
		t.Error("expected migrations to be recorded in schema_migrations")
	}
}

func TestMigrationRunnerIdempotent(t *testing.T) {
	// Run twice - should not error
	db, _ := sql.Open("sqlite3", ":memory:")
	defer db.Close()

	log := slog.New(slog.NewTextHandler(nil, nil))
	runner := NewMigrationRunner(log, db)

	ctx := context.Background()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/store -v
```

Expected: All tests pass, migrations load and execute correctly.

- [ ] **Step 6: Commit**

```bash
git add go/assets/migrations/ go/internal/store/migration_runner.go go/internal/store/migrations_test.go
git commit -m "refactor: migrate from monolithic to stacked SQL migrations"
```

---

## Benefits After Refactoring

✅ **Maintainability:** Each schema change is isolated to its own file
✅ **Version tracking:** Know exactly which schema version is deployed
✅ **Incremental updates:** Add new migrations without modifying old ones
✅ **Testability:** Test each migration independently
✅ **Extensibility:** Easy to add migrations for Phase 5+ features
✅ **Git clarity:** Clear commit history for schema evolution
✅ **Rollback capability:** (Phase 6+) Can roll back by removing migration records

---

## Future Enhancements (Phase 6+)

1. **Rollback mechanism:** Track rollback SQL for each migration
2. **Migration validation:** Test migrations on staging before production
3. **Schema documentation:** Auto-generate schema docs from migrations
4. **Migration ordering:** Handle complex dependencies between migrations
5. **Data migrations:** Support data transformation migrations (not just DDL)

---

## Verification Checklist

- [ ] All migration files created in `go/assets/migrations/`
- [ ] Migration runner loads and executes files
- [ ] Version tracking table created and populated
- [ ] Idempotent: Re-running doesn't break existing schemas
- [ ] Tests pass: `go test ./internal/store -v`
- [ ] No build warnings
- [ ] Old monolithic migrations.go completely removed
- [ ] Database operations work as before (backward compatible)

---

## Expected Deliverables

- ✅ `go/assets/migrations/000_schema_migrations.sql` (migration tracker)
- ✅ `go/assets/migrations/001_init.sql` (initial schema)
- ✅ `go/assets/migrations/002_add_pr_indexes.sql` (performance)
- ✅ `go/assets/migrations/003_future_placeholder.sql` (template)
- ✅ `go/internal/store/migration_runner.go` (~200 LOC)
- ✅ `go/internal/store/migrations_test.go` (~60 LOC)
- ✅ Updated `go/internal/store/db.go`
- ✅ 3 tests covering loading, execution, idempotency
- ✅ Removed old monolithic code
- ✅ Full backward compatibility

---

## Estimated Effort

- **Phase 5 completion:** Continue with Actions implementation
- **After Phase 5:** 2-3 hours to implement migration refactoring
- **Impact:** Enables scalable schema versioning for Phase 6+

---

**This refactoring should be completed after Phase 5 to avoid blocking ongoing feature work.**
