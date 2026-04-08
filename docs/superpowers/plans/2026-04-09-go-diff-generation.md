# Go Git Diff Generation & WebSocket Serving — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist PR diffs to the database during sync, add a `REQUEST_DIFF` WebSocket handler so Electron can fetch diffs on demand, and wire the full flow: sync generates diffs → stores in DB → WebSocket serves them to the UI.

**Architecture:** The Go sync engine already has `git.Client.GenerateDiff()` which produces per-file patches via `go-git`. The orchestrator calls it in `FetchPRDetails` when `IncludeDiff=true` but only stores `DiffSize`, not the actual patches. This plan: (1) adds a `pr_diffs` table, (2) persists file patches during sync, (3) adds `REQUEST_DIFF` to the WebSocket dispatcher, (4) returns the unified diff to Electron.

**Tech Stack:** Go, SQLite, go-git, WebSocket

---

## File Structure

| File | Responsibility |
|---|---|
| **Create:** `go/internal/db/migrations/004_pr_diffs.go` | Migration to add `pr_diffs` table |
| **Create:** `go/internal/db/diff_store.go` | CRUD for pr_diffs table |
| **Create:** `go/internal/db/diff_store_test.go` | Tests for diff persistence |
| **Create:** `go/internal/ws/handlers/diff_handler.go` | REQUEST_DIFF WebSocket handler |
| **Create:** `go/internal/ws/handlers/diff_handler_test.go` | Tests for diff handler |
| **Modify:** `go/internal/sync/orchestrator.go` | Persist diff results to DB after generation |
| **Modify:** `go/internal/ws/server.go` | Register REQUEST_DIFF handler in dispatcher |
| **Modify:** `go/internal/server/app.go` | Wire diff store and handler |

---

### Task 1: Database Migration — pr_diffs Table

**Files:**
- Create: `go/internal/db/migrations/004_pr_diffs.go`
- Test: Build + migrate

- [ ] **Step 1: Check existing migration pattern**

Read `go/internal/db/migrations/` to understand the migration runner pattern.

- [ ] **Step 2: Create the migration**

Create `go/internal/db/migrations/004_pr_diffs.go` (or the appropriate numbered file following the existing pattern):

```go
package migrations

func init() {
    Register(4, Migration{
        Name: "create_pr_diffs",
        Up: `CREATE TABLE IF NOT EXISTS pr_diffs (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            pr_id TEXT NOT NULL,
            file_path TEXT NOT NULL,
            patch TEXT NOT NULL,
            additions INTEGER NOT NULL DEFAULT 0,
            deletions INTEGER NOT NULL DEFAULT 0,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(pr_id, file_path)
        );
        CREATE INDEX IF NOT EXISTS idx_pr_diffs_pr_id ON pr_diffs(pr_id);`,
        Down: `DROP TABLE IF EXISTS pr_diffs;`,
    })
}
```

Adapt to the exact migration runner API used in the project (check if it uses `Register`, a slice, or `init()` pattern).

- [ ] **Step 3: Run build to verify**

Run: `cd go && go build ./...`
Expected: Compiles

- [ ] **Step 4: Run tests**

Run: `cd go && go test ./internal/db/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/internal/db/migrations/
git commit -m "feat: add pr_diffs table migration for storing PR diff patches"
```

---

### Task 2: Diff Store — CRUD Operations

**Files:**
- Create: `go/internal/db/diff_store.go`
- Create: `go/internal/db/diff_store_test.go`

- [ ] **Step 1: Write failing tests**

Create `go/internal/db/diff_store_test.go`:

```go
func TestDiffStore_SaveAndGet(t *testing.T) {
    db := setupTestDB(t) // Use existing test helper
    store := NewDiffStore(db)

    diffs := []DiffFile{
        {FilePath: "main.go", Patch: "+added line\n-removed line", Additions: 1, Deletions: 1},
        {FilePath: "utils.go", Patch: "+new file", Additions: 1, Deletions: 0},
    }

    err := store.SaveDiffs("pr-123", diffs)
    require.NoError(t, err)

    got, err := store.GetDiffs("pr-123")
    require.NoError(t, err)
    assert.Len(t, got, 2)
    assert.Equal(t, "main.go", got[0].FilePath)
    assert.Equal(t, "+added line\n-removed line", got[0].Patch)
}

func TestDiffStore_GetUnifiedDiff(t *testing.T) {
    db := setupTestDB(t)
    store := NewDiffStore(db)

    diffs := []DiffFile{
        {FilePath: "main.go", Patch: "--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n-old\n+new"},
    }
    store.SaveDiffs("pr-456", diffs)

    unified, err := store.GetUnifiedDiff("pr-456")
    require.NoError(t, err)
    assert.Contains(t, unified, "diff --git a/main.go b/main.go")
}

func TestDiffStore_Upsert(t *testing.T) {
    db := setupTestDB(t)
    store := NewDiffStore(db)

    store.SaveDiffs("pr-789", []DiffFile{{FilePath: "a.go", Patch: "v1", Additions: 1, Deletions: 0}})
    store.SaveDiffs("pr-789", []DiffFile{{FilePath: "a.go", Patch: "v2", Additions: 2, Deletions: 1}})

    got, err := store.GetDiffs("pr-789")
    require.NoError(t, err)
    assert.Len(t, got, 1)
    assert.Equal(t, "v2", got[0].Patch)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd go && go test ./internal/db/ -run TestDiffStore -v`
Expected: FAIL — types not found

- [ ] **Step 3: Implement DiffStore**

Create `go/internal/db/diff_store.go`:

```go
package db

import "database/sql"

type DiffFile struct {
    FilePath  string
    Patch     string
    Additions int
    Deletions int
}

type DiffStore struct {
    db *sql.DB
}

func NewDiffStore(db *sql.DB) *DiffStore {
    return &DiffStore{db: db}
}

func (s *DiffStore) SaveDiffs(prID string, diffs []DiffFile) error {
    tx, err := s.db.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(pr_id, file_path) DO UPDATE SET
            patch = excluded.patch,
            additions = excluded.additions,
            deletions = excluded.deletions,
            created_at = CURRENT_TIMESTAMP
    `)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, d := range diffs {
        if _, err := stmt.Exec(prID, d.FilePath, d.Patch, d.Additions, d.Deletions); err != nil {
            return err
        }
    }

    return tx.Commit()
}

func (s *DiffStore) GetDiffs(prID string) ([]DiffFile, error) {
    rows, err := s.db.Query(
        "SELECT file_path, patch, additions, deletions FROM pr_diffs WHERE pr_id = ? ORDER BY file_path",
        prID,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var diffs []DiffFile
    for rows.Next() {
        var d DiffFile
        if err := rows.Scan(&d.FilePath, &d.Patch, &d.Additions, &d.Deletions); err != nil {
            return nil, err
        }
        diffs = append(diffs, d)
    }
    return diffs, rows.Err()
}

// GetUnifiedDiff returns all file patches concatenated as a single unified diff string.
func (s *DiffStore) GetUnifiedDiff(prID string) (string, error) {
    diffs, err := s.GetDiffs(prID)
    if err != nil {
        return "", err
    }

    var unified string
    for _, d := range diffs {
        unified += "diff --git a/" + d.FilePath + " b/" + d.FilePath + "\n"
        unified += d.Patch + "\n"
    }
    return unified, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go && go test ./internal/db/ -run TestDiffStore -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/internal/db/diff_store.go go/internal/db/diff_store_test.go
git commit -m "feat: add DiffStore for persisting and querying PR diffs"
```

---

### Task 3: Persist Diffs During Sync

**Files:**
- Modify: `go/internal/sync/orchestrator.go`

- [ ] **Step 1: Read orchestrator.go to find FetchPRDetails**

Read the file to understand where `GenerateDiff` is called and where results are used.

- [ ] **Step 2: Wire DiffStore into orchestrator**

After `GenerateDiff` returns `diffResult`, persist the files:

```go
    // After generating diff:
    if o.diffStore != nil && diffResult != nil {
        var diffFiles []db.DiffFile
        for _, f := range diffResult.Files {
            diffFiles = append(diffFiles, db.DiffFile{
                FilePath:  f.Path,
                Patch:     f.Patch,
                Additions: f.Added,
                Deletions: f.Removed,
            })
        }
        if err := o.diffStore.SaveDiffs(pr.ID, diffFiles); err != nil {
            o.logger.Error("failed to persist diffs", "pr_id", pr.ID, "error", err)
        }
    }
```

Add `diffStore *db.DiffStore` to the orchestrator struct and constructor.

- [ ] **Step 3: Run Go tests**

Run: `cd go && go test ./internal/sync/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add go/internal/sync/orchestrator.go
git commit -m "feat: persist PR diffs to database during sync"
```

---

### Task 4: REQUEST_DIFF WebSocket Handler

**Files:**
- Create: `go/internal/ws/handlers/diff_handler.go`
- Modify: `go/internal/ws/server.go` or `go/internal/server/app.go`

- [ ] **Step 1: Create diff handler**

Create `go/internal/ws/handlers/diff_handler.go`:

```go
package handlers

import (
    "encoding/json"
    "fmt"

    "bistro/internal/db"
    "bistro/internal/ws"
)

type DiffHandler struct {
    store *db.DiffStore
}

func NewDiffHandler(store *db.DiffStore) *DiffHandler {
    return &DiffHandler{store: store}
}

func (h *DiffHandler) Handle(msg ws.InboundMessage) (interface{}, error) {
    var data struct {
        PRID string `json:"pr_id"`
        Repo string `json:"repo"`
    }
    if err := json.Unmarshal(msg.Data, &data); err != nil {
        return nil, fmt.Errorf("invalid REQUEST_DIFF data: %w", err)
    }

    diff, err := h.store.GetUnifiedDiff(data.PRID)
    if err != nil {
        return map[string]interface{}{
            "id":   msg.ID,
            "type": "DIFF_READY",
            "data": map[string]interface{}{
                "diff":  "",
                "error": err.Error(),
            },
        }, nil
    }

    return map[string]interface{}{
        "id":   msg.ID,
        "type": "DIFF_READY",
        "data": map[string]interface{}{
            "diff": diff,
        },
    }, nil
}
```

- [ ] **Step 2: Register the handler in the dispatcher**

In `app.go` or wherever the dispatcher is created:

```go
    diffHandler := handlers.NewDiffHandler(diffStore)
    dispatcher.Register("REQUEST_DIFF", diffHandler.Handle)
```

- [ ] **Step 3: Run Go tests**

Run: `cd go && go test ./...`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add go/internal/ws/handlers/ go/internal/server/app.go
git commit -m "feat: add REQUEST_DIFF WebSocket handler serving diffs from database"
```

---

### Task 5: Build Verification

- [ ] **Step 1: Run Go tests**

Run: `cd go && go test ./...`
Expected: All pass

- [ ] **Step 2: Build Go binary**

Run: `cd go && go build -o bistro-sync ./cmd/bistro-sync`
Expected: Clean build

- [ ] **Step 3: Run Electron tests + build**

Run: `cd electron && pnpm test && pnpm build`
Expected: All pass (Electron should be unaffected)

- [ ] **Step 4: Final commit**

```bash
git add -A && git commit -m "feat: Go diff generation — build verification"
```
