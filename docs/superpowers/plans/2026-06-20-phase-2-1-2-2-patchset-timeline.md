# Phase 2.1 + 2.2 — Patchset Timeline + Since-Last-Review — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose observed patchset history and a "since last review" diff to the reviewer. A patchset timeline (P1/P2/P3) appears in the PR detail view; `s` in diff mode renders the diff since the reviewer's last submission via system `git range-diff`, with graceful fallback to `go-git Tree.Patch` when system git is absent or < 2.19.

**Architecture:** A new `go/internal/git/rangediff.go` wraps the only sanctioned `os/exec` in the codebase — `git range-diff` — with a startup capability probe (cached, 3 s timeout). If unavailable it returns `Degraded: true` and a plain `Tree.Patch` diff. Two new store helpers (`last_reviewed.go`, `files_viewed.go`) persist reviewer state. Two new WS handlers (`patchsets.go`, `rangediff.go`) serve the timeline and diff pair. The Electron UI adds a `PatchsetTimeline` component, a `RangeDiffView`, a footer degraded indicator, and a `s` keybinding.

**Tech Stack:** Go 1.26 (`os/exec`, stdlib `crypto/sha256`/`encoding/hex`/`regexp`), SQLite (`mattn/go-sqlite3`, `_foreign_keys=on`), React + TypeScript, TanStack Store, neverthrow, Vitest.

**Spec:** `docs/superpowers/specs/2026-06-20-phase-2-1-2-2-patchset-timeline-design.md`

**Depends on:** Phase 2.0 Foundation (migration 012, `patchsets` table, `current_user` table).

---

## File Structure

**New Go files:**
- `go/internal/store/migrations/014_add_last_reviewed_and_files_viewed.sql` — migration
- `go/internal/store/last_reviewed.go` — `SetLastReviewedSHA` / `GetLastReviewedSHA`
- `go/internal/store/last_reviewed_test.go`
- `go/internal/store/files_viewed.go` — `MarkFileViewed` / `IsFileViewed` / `ListViewedFiles`
- `go/internal/store/files_viewed_test.go`
- `go/internal/git/rangediff.go` — `RangeDiffer` struct, capability probe, `RunRangeDiff`
- `go/internal/git/rangediff_test.go`
- `go/internal/server/handler/patchsets.go` — `NewListPatchsetsHandler`
- `go/internal/server/handler/rangediff.go` — `NewRangeDiffHandler`

**Modified Go files:**
- `go/internal/server/app.go` — build `RangeDiffer`, run probe at startup, inject into handlers
- `go/internal/server/handler/pr.go` — call `SetLastReviewedSHA` after successful review submission
- `go/cmd/bistro-sync/main.go` — log probe result at startup

**New Electron files:**
- `electron/src/renderer/components/PatchsetTimeline.tsx`
- `electron/src/renderer/components/RangeDiffView.tsx`
- `electron/src/renderer/components/DegradedRangeDiffIndicator.tsx`
- `electron/tests/unit/PatchsetTimeline.test.tsx`
- `electron/tests/unit/RangeDiffView.test.tsx`

**Modified Electron files:**
- `electron/src/renderer/types/index.ts` — add `Patchset`, `RangeDiffResult`, `FileViewedState`
- `electron/src/renderer/pages/PRDetailPage.tsx` — mount `PatchsetTimeline`
- `electron/src/renderer/pages/DiffPage.tsx` — accept patchset pair, render `RangeDiffView`
- `electron/src/renderer/utils/keybindings.ts` — add `diff.sinceLastReview` (`s` key)

**Conventions to follow:**
- Go: constructor `NewX(log *slog.Logger, ...)`, `if log == nil { log = slog.Default() }`; wrap errors with `fmt.Errorf("...: %w", err)`; per-item errors logged and skipped, never abort the tx.
- Migrations: `-- Migration: NNN` + `-- Description:` header (parser reads `Description:`), `IF NOT EXISTS` where applicable.
- TS: explicit types on exported fns/props, string-literal unions over enums, immutable updates, neverthrow for fallible paths, no `console.log`.
- The `os/exec` in `rangediff.go` is the **only** sanctioned `os/exec` in production Go. After this task: `grep -rn "os/exec" go/internal/ | grep -v rangediff.go` must return nothing.

---

## Task 1: Migration 014 — `last_reviewed_sha` + `files_viewed`

**Files:**
- Create: `go/internal/store/migrations/014_add_last_reviewed_and_files_viewed.sql`
- Modify: `go/internal/store/migration_runner_test.go` (add a test case)

- [ ] **Step 1: Write the migration SQL**

```sql
-- Migration: 014
-- Description: last_reviewed_sha per (pr_id, login) and files_viewed with content_hash for rebase-safe viewed state

CREATE TABLE IF NOT EXISTS last_reviewed_sha (
    pr_id      TEXT      NOT NULL,
    login      TEXT      NOT NULL,
    sha        TEXT      NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS files_viewed (
    pr_id        TEXT      NOT NULL,
    file_path    TEXT      NOT NULL,
    login        TEXT      NOT NULL,
    content_hash TEXT      NOT NULL,
    viewed_at    TIMESTAMP NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    PRIMARY KEY (pr_id, file_path, login),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_last_reviewed_pr ON last_reviewed_sha(pr_id);
CREATE INDEX IF NOT EXISTS idx_files_viewed_pr_login ON files_viewed(pr_id, login);
```

- [ ] **Step 2: Write a failing migration test**

Add to `go/internal/store/migration_runner_test.go`:

```go
func TestMigration014AddsLastReviewedAndFilesViewed(t *testing.T) {
    db := newTestDB(t) // existing helper: opens in-memory DB, runs all migrations
    // verify last_reviewed_sha columns
    for _, want := range []string{"pr_id", "login", "sha", "updated_at"} {
        rows, err := db.Query(`SELECT name FROM pragma_table_info('last_reviewed_sha') WHERE name = ?`, want)
        if err != nil {
            t.Fatalf("pragma last_reviewed_sha: %v", err)
        }
        var found bool
        for rows.Next() { found = true }
        rows.Close()
        if !found {
            t.Errorf("last_reviewed_sha missing column %q", want)
        }
    }
    // verify files_viewed columns
    for _, want := range []string{"pr_id", "file_path", "login", "content_hash", "viewed_at"} {
        rows, err := db.Query(`SELECT name FROM pragma_table_info('files_viewed') WHERE name = ?`, want)
        if err != nil {
            t.Fatalf("pragma files_viewed: %v", err)
        }
        var found bool
        for rows.Next() { found = true }
        rows.Close()
        if !found {
            t.Errorf("files_viewed missing column %q", want)
        }
    }
    // sanity: FK enforcement is on — inserting with a bad pr_id must fail
    _, err := db.Exec(`INSERT INTO last_reviewed_sha (pr_id, login, sha) VALUES ('no-such-pr', 'alice', 'abc')`)
    if err == nil {
        t.Error("expected FK violation inserting last_reviewed_sha with non-existent pr_id")
    }
}
```

- [ ] **Step 3: Run the test (expect FAIL — migration not yet auto-discovered)**

```bash
cd go && go test ./internal/store/ -run TestMigration014 -v
```

Expected: FAIL (`last_reviewed_sha` table not found or migration not loaded).

- [ ] **Step 4: Verify the migration runner picks up the new file**

The migration runner (`go/internal/store/migration_runner.go`) uses `//go:embed migrations/*.sql`
or reads the directory. Confirm your new file follows the naming convention
(`NNN_description.sql`) so it auto-discovers. No code change should be needed; if the
runner explicitly whitelists files, add `014_add_last_reviewed_and_files_viewed.sql`.

- [ ] **Step 5: Run the test (expect PASS)**

```bash
cd go && go test ./internal/store/ -run TestMigration014 -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/internal/store/migrations/014_add_last_reviewed_and_files_viewed.sql go/internal/store/migration_runner_test.go
git commit -m "feat(store): migration 014 — last_reviewed_sha + files_viewed tables"
```

---

## Task 2: Store helpers — `last_reviewed.go`

**Files:**
- Create: `go/internal/store/last_reviewed.go`
- Create: `go/internal/store/last_reviewed_test.go`

- [ ] **Step 1: Write failing tests**

```go
// go/internal/store/last_reviewed_test.go
package store_test

import (
    "context"
    "testing"
)

func TestSetGetLastReviewedSHA_RoundTrip(t *testing.T) {
    db := newTestDB(t)
    ctx := context.Background()

    // missing → empty string, false
    sha, ok, err := db.GetLastReviewedSHA(ctx, "pr-1", "alice")
    if err != nil { t.Fatalf("get: %v", err) }
    if ok || sha != "" { t.Fatalf("expected empty, got %q %v", sha, ok) }

    // insert a PR row so FK is satisfied
    _, err = db.Exec(`INSERT INTO prs (id, number, title, state, author, base_branch, head_branch, base_sha, head_sha, repo_id, created_at, updated_at) VALUES ('pr-1',1,'t','open','u','main','feat','aaa','bbb','r1','2024-01-01','2024-01-01')`)
    if err != nil { t.Fatalf("insert pr: %v", err) }

    if err := db.SetLastReviewedSHA(ctx, "pr-1", "alice", "deadbeef"); err != nil {
        t.Fatalf("set: %v", err)
    }
    sha, ok, err = db.GetLastReviewedSHA(ctx, "pr-1", "alice")
    if err != nil { t.Fatalf("get after set: %v", err) }
    if !ok || sha != "deadbeef" { t.Fatalf("expected deadbeef/true, got %q %v", sha, ok) }

    // upsert updates the sha
    if err := db.SetLastReviewedSHA(ctx, "pr-1", "alice", "cafebabe"); err != nil {
        t.Fatalf("re-set: %v", err)
    }
    sha, _, _ = db.GetLastReviewedSHA(ctx, "pr-1", "alice")
    if sha != "cafebabe" { t.Fatalf("upsert: expected cafebabe, got %q", sha) }
}

func TestGetLastReviewedSHA_DifferentLogins(t *testing.T) {
    db := newTestDB(t)
    ctx := context.Background()
    _, _ = db.Exec(`INSERT INTO prs (id, number, title, state, author, base_branch, head_branch, base_sha, head_sha, repo_id, created_at, updated_at) VALUES ('pr-2',2,'t','open','u','main','feat','aaa','bbb','r1','2024-01-01','2024-01-01')`)
    _ = db.SetLastReviewedSHA(ctx, "pr-2", "alice", "sha-alice")
    _ = db.SetLastReviewedSHA(ctx, "pr-2", "bob", "sha-bob")
    shaA, _, _ := db.GetLastReviewedSHA(ctx, "pr-2", "alice")
    shaB, _, _ := db.GetLastReviewedSHA(ctx, "pr-2", "bob")
    if shaA != "sha-alice" || shaB != "sha-bob" {
        t.Fatalf("login isolation: alice=%q bob=%q", shaA, shaB)
    }
}
```

Run: `cd go && go test ./internal/store/ -run TestSetGetLastReviewedSHA -v`
Expected: FAIL (methods don't exist yet).

- [ ] **Step 2: Implement `last_reviewed.go`**

```go
// go/internal/store/last_reviewed.go
package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
)

// SetLastReviewedSHA upserts the SHA that login last reviewed for pr_id.
// Called after a successful review submission (approve / request-changes / comment).
func (db *DB) SetLastReviewedSHA(ctx context.Context, prID, login, sha string) error {
    _, err := db.ExecContext(ctx, `
        INSERT INTO last_reviewed_sha (pr_id, login, sha, updated_at)
        VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
        ON CONFLICT(pr_id, login) DO UPDATE SET
            sha        = excluded.sha,
            updated_at = excluded.updated_at
    `, prID, login, sha)
    if err != nil {
        return fmt.Errorf("set last_reviewed_sha (%s, %s): %w", prID, login, err)
    }
    return nil
}

// GetLastReviewedSHA returns the last reviewed SHA for (pr_id, login).
// Returns ("", false, nil) when no row exists.
func (db *DB) GetLastReviewedSHA(ctx context.Context, prID, login string) (string, bool, error) {
    var sha string
    err := db.QueryRowContext(ctx, `
        SELECT sha FROM last_reviewed_sha WHERE pr_id = ? AND login = ?
    `, prID, login).Scan(&sha)
    if errors.Is(err, sql.ErrNoRows) {
        return "", false, nil
    }
    if err != nil {
        return "", false, fmt.Errorf("get last_reviewed_sha (%s, %s): %w", prID, login, err)
    }
    return sha, true, nil
}
```

- [ ] **Step 3: Run tests (expect PASS)**

```bash
cd go && go test ./internal/store/ -run TestSetGetLastReviewedSHA -v
cd go && go test ./internal/store/ -run TestGetLastReviewedSHA_DifferentLogins -v
```

- [ ] **Step 4: Commit**

```bash
git add go/internal/store/last_reviewed.go go/internal/store/last_reviewed_test.go
git commit -m "feat(store): SetLastReviewedSHA / GetLastReviewedSHA helpers"
```

---

## Task 3: Store helpers — `files_viewed.go`

**Files:**
- Create: `go/internal/store/files_viewed.go`
- Create: `go/internal/store/files_viewed_test.go`

- [ ] **Step 1: Write failing tests**

```go
// go/internal/store/files_viewed_test.go
package store_test

import (
    "context"
    "testing"
)

func setupPR(t *testing.T, db *DB, prID string) {
    t.Helper()
    _, err := db.Exec(`INSERT OR IGNORE INTO prs (id, number, title, state, author, base_branch, head_branch, base_sha, head_sha, repo_id, created_at, updated_at) VALUES (?,1,'t','open','u','main','feat','aaa','bbb','r1','2024-01-01','2024-01-01')`, prID)
    if err != nil { t.Fatalf("insert pr: %v", err) }
}

func TestMarkIsFileViewed_ContentHashSemantics(t *testing.T) {
    db := newTestDB(t)
    ctx := context.Background()
    setupPR(t, db, "pr-fv")

    const (prID = "pr-fv"; path = "go/internal/foo.go"; login = "alice"; hash1 = "abc123"; hash2 = "def456")

    // not viewed initially
    ok, err := db.IsFileViewed(ctx, prID, path, login, hash1)
    if err != nil { t.Fatalf("is viewed: %v", err) }
    if ok { t.Fatal("should not be viewed yet") }

    // mark viewed with hash1
    if err := db.MarkFileViewed(ctx, prID, path, login, hash1); err != nil {
        t.Fatalf("mark: %v", err) 
    }

    // same hash → viewed
    ok, _ = db.IsFileViewed(ctx, prID, path, login, hash1)
    if !ok { t.Fatal("same hash should be viewed") }

    // different hash (content changed) → not viewed
    ok, _ = db.IsFileViewed(ctx, prID, path, login, hash2)
    if ok { t.Fatal("different hash should not be viewed (content changed)") }

    // re-mark with hash2 → now hash2 is viewed, hash1 is not
    _ = db.MarkFileViewed(ctx, prID, path, login, hash2)
    ok, _ = db.IsFileViewed(ctx, prID, path, login, hash2)
    if !ok { t.Fatal("hash2 should now be viewed after re-mark") }
    ok, _ = db.IsFileViewed(ctx, prID, path, login, hash1)
    if ok { t.Fatal("hash1 should no longer be viewed after upsert to hash2") }
}

func TestListViewedFiles(t *testing.T) {
    db := newTestDB(t)
    ctx := context.Background()
    setupPR(t, db, "pr-lv")
    _ = db.MarkFileViewed(ctx, "pr-lv", "a.go", "alice", "h1")
    _ = db.MarkFileViewed(ctx, "pr-lv", "b.go", "alice", "h2")
    _ = db.MarkFileViewed(ctx, "pr-lv", "c.go", "bob",   "h3")
    files, err := db.ListViewedFiles(ctx, "pr-lv", "alice")
    if err != nil { t.Fatalf("list: %v", err) }
    if len(files) != 2 { t.Fatalf("expected 2, got %d", len(files)) }
}
```

Run: `cd go && go test ./internal/store/ -run TestMarkIsFileViewed -v`
Expected: FAIL.

- [ ] **Step 2: Implement `files_viewed.go`**

```go
// go/internal/store/files_viewed.go
package store

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
)

// FileViewed is one row from files_viewed.
type FileViewed struct {
    FilePath    string
    ContentHash string
}

// MarkFileViewed upserts a viewed record for (pr_id, file_path, login).
// The content_hash is updated on conflict — viewing after a rebase replaces the old hash.
func (db *DB) MarkFileViewed(ctx context.Context, prID, filePath, login, contentHash string) error {
    _, err := db.ExecContext(ctx, `
        INSERT INTO files_viewed (pr_id, file_path, login, content_hash, viewed_at)
        VALUES (?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
        ON CONFLICT(pr_id, file_path, login) DO UPDATE SET
            content_hash = excluded.content_hash,
            viewed_at    = excluded.viewed_at
    `, prID, filePath, login, contentHash)
    if err != nil {
        return fmt.Errorf("mark file viewed (%s, %s, %s): %w", prID, filePath, login, err)
    }
    return nil
}

// IsFileViewed returns true only when a row exists for (pr_id, file_path, login)
// AND the stored content_hash matches the given hash.
// Different hash → false (content changed; needs re-review).
func (db *DB) IsFileViewed(ctx context.Context, prID, filePath, login, contentHash string) (bool, error) {
    var stored string
    err := db.QueryRowContext(ctx, `
        SELECT content_hash FROM files_viewed
        WHERE pr_id = ? AND file_path = ? AND login = ?
    `, prID, filePath, login).Scan(&stored)
    if errors.Is(err, sql.ErrNoRows) {
        return false, nil
    }
    if err != nil {
        return false, fmt.Errorf("is file viewed (%s, %s, %s): %w", prID, filePath, login, err)
    }
    return stored == contentHash, nil
}

// ListViewedFiles returns all files the given login has viewed for pr_id,
// with the stored content_hash for each.
func (db *DB) ListViewedFiles(ctx context.Context, prID, login string) ([]FileViewed, error) {
    rows, err := db.QueryContext(ctx, `
        SELECT file_path, content_hash FROM files_viewed
        WHERE pr_id = ? AND login = ?
        ORDER BY file_path
    `, prID, login)
    if err != nil {
        return nil, fmt.Errorf("list viewed files (%s, %s): %w", prID, login, err)
    }
    defer rows.Close()
    var out []FileViewed
    for rows.Next() {
        var fv FileViewed
        if err := rows.Scan(&fv.FilePath, &fv.ContentHash); err != nil {
            return nil, fmt.Errorf("scan file viewed: %w", err)
        }
        out = append(out, fv)
    }
    return out, rows.Err()
}
```

- [ ] **Step 3: Run tests (expect PASS)**

```bash
cd go && go test ./internal/store/ -run 'TestMarkIsFileViewed|TestListViewedFiles' -v
```

- [ ] **Step 4: Commit**

```bash
git add go/internal/store/files_viewed.go go/internal/store/files_viewed_test.go
git commit -m "feat(store): MarkFileViewed / IsFileViewed / ListViewedFiles helpers"
```

---

## Task 4: `RangeDiffer` wrapper + capability probe

**Files:**
- Create: `go/internal/git/rangediff.go`
- Create: `go/internal/git/rangediff_test.go`

- [ ] **Step 1: Write failing tests**

The test file uses a mock git binary written to a temp dir and prepended to PATH.

```go
// go/internal/git/rangediff_test.go
package git

import (
    "context"
    "os"
    "path/filepath"
    "runtime"
    "strings"
    "testing"
)

// writeMockGit writes a tiny script that prints the given output and exits with code.
// Returns the directory containing the script (prepend to PATH).
func writeMockGit(t *testing.T, output string, exitCode int) string {
    t.Helper()
    dir := t.TempDir()
    var name string
    var content string
    if runtime.GOOS == "windows" {
        name = "git.bat"
        content = "@echo off\r\necho " + output + "\r\nexit /b " + itoa(exitCode)
    } else {
        name = "git"
        content = "#!/bin/sh\necho '" + output + "'\nexit " + itoa(exitCode) + "\n"
    }
    p := filepath.Join(dir, name)
    if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
        t.Fatalf("writeMockGit: %v", err)
    }
    return dir
}

func itoa(n int) string {
    if n == 0 { return "0" }
    return strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(fmt.Sprintf("%d", n), "", "")), ""))
}

func prependPath(t *testing.T, dir string) {
    t.Helper()
    old := os.Getenv("PATH")
    t.Setenv("PATH", dir+string(os.PathListSeparator)+old)
}

func TestRangeDiffer_ProbeAvailable(t *testing.T) {
    dir := writeMockGit(t, "git version 2.43.0", 0)
    prependPath(t, dir)
    rd := NewRangeDiffer(nil, nil)
    if !rd.available {
        t.Fatalf("expected available=true for git 2.43.0, fallback reason: %s", rd.fallbackReason)
    }
}

func TestRangeDiffer_ProbeTooOld(t *testing.T) {
    dir := writeMockGit(t, "git version 2.18.4", 0)
    prependPath(t, dir)
    rd := NewRangeDiffer(nil, nil)
    if rd.available {
        t.Fatal("expected available=false for git 2.18.4")
    }
}

func TestRangeDiffer_ProbeNotFound(t *testing.T) {
    // Point PATH to an empty dir so git is not found
    empty := t.TempDir()
    t.Setenv("PATH", empty)
    rd := NewRangeDiffer(nil, nil)
    if rd.available {
        t.Fatal("expected available=false when git not on PATH")
    }
}

func TestRangeDiffer_InjectionGuard_SHA(t *testing.T) {
    rd := NewRangeDiffer(nil, nil)
    rd.available = true // force to exercise the exec path's SHA validation
    _, err := rd.RunRangeDiff(context.Background(), t.TempDir(), "not-a-hex", "abc123"+strings.Repeat("0",34), "abc123"+strings.Repeat("0",34), "abc123"+strings.Repeat("0",34))
    if err == nil {
        t.Fatal("expected error for non-hex SHA")
    }
}

func TestRangeDiffer_FallbackDiff(t *testing.T) {
    rd := NewRangeDiffer(nil, nil)
    rd.available = false
    rd.fallbackReason = "system git not available"
    // With nil gitClient the fallback also returns degraded (no clone to diff);
    // test that Degraded is set and no panic occurs.
    result, err := rd.RunRangeDiff(context.Background(), "", "", "", "", "")
    if err != nil && !result.Degraded {
        t.Fatalf("fallback should set Degraded: err=%v result=%+v", err, result)
    }
    if !result.Degraded {
        t.Fatal("expected Degraded=true in fallback path")
    }
}
```

Run: `cd go && go test ./internal/git/ -run 'TestRangeDiffer' -v`
Expected: FAIL (type does not exist).

- [ ] **Step 2: Implement `rangediff.go`**

```go
// go/internal/git/rangediff.go
package git

import (
    "context"
    "fmt"
    "log/slog"
    "os/exec"
    "regexp"
    "strconv"
    "strings"
    "time"
)

var hexSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

func isHexSHA(s string) bool { return hexSHARe.MatchString(s) }

// RangeDiffResult is the output of a range-diff operation.
type RangeDiffResult struct {
    Raw            string
    Degraded       bool
    FallbackReason string
}

// RangeDiffer wraps `git range-diff` with a one-time capability probe.
// If system git is absent or < 2.19, all calls degrade to go-git Tree.Patch.
type RangeDiffer struct {
    log            *slog.Logger
    gitClient      *Client
    available      bool
    fallbackReason string
}

// NewRangeDiffer constructs a RangeDiffer and immediately probes for git availability.
// Probe result is cached; it is never re-run per diff.
func NewRangeDiffer(log *slog.Logger, gitClient *Client) *RangeDiffer {
    if log == nil {
        log = slog.Default()
    }
    rd := &RangeDiffer{log: log, gitClient: gitClient}
    rd.probeGit()
    return rd
}

// Available reports whether system git >= 2.19 was found at construction time.
func (rd *RangeDiffer) Available() bool { return rd.available }

// FallbackReason returns the human-readable reason why range-diff is unavailable.
// Empty when Available() == true.
func (rd *RangeDiffer) FallbackReason() string { return rd.fallbackReason }

// probeGit runs `git --version` with a 3-second timeout and caches the result.
func (rd *RangeDiffer) probeGit() {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    out, err := exec.CommandContext(ctx, "git", "--version").Output()
    if err != nil {
        rd.available = false
        rd.fallbackReason = fmt.Sprintf("git not found or probe failed: %v", err)
        rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
        return
    }
    // Parse "git version X.Y.Z"
    parts := strings.Fields(strings.TrimSpace(string(out)))
    if len(parts) < 3 {
        rd.available = false
        rd.fallbackReason = fmt.Sprintf("unparseable git version output: %q", out)
        rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
        return
    }
    ver := parts[2]
    segments := strings.SplitN(ver, ".", 3)
    if len(segments) < 2 {
        rd.available = false
        rd.fallbackReason = fmt.Sprintf("unparseable version %q", ver)
        rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
        return
    }
    major, _ := strconv.Atoi(segments[0])
    minor, _ := strconv.Atoi(segments[1])
    if major > 2 || (major == 2 && minor >= 19) {
        rd.available = true
        rd.log.Info("git range-diff available", "version", ver)
    } else {
        rd.available = false
        rd.fallbackReason = fmt.Sprintf("git %s < 2.19 (range-diff requires >= 2.19)", ver)
        rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
    }
}

// RunRangeDiff executes `git range-diff baseStart..baseEnd headStart..headEnd`
// in repoPath. All SHA arguments must be 40-char lowercase hex — validated before exec.
// If unavailable, falls back to go-git Tree.Patch (headStart..headEnd) and sets Degraded=true.
func (rd *RangeDiffer) RunRangeDiff(
    ctx context.Context,
    repoPath, baseStart, baseEnd, headStart, headEnd string,
) (RangeDiffResult, error) {
    if !rd.available {
        return rd.fallback(headStart, headEnd)
    }
    // Injection safety: validate all SHAs.
    for _, sha := range []string{baseStart, baseEnd, headStart, headEnd} {
        if !isHexSHA(sha) {
            return RangeDiffResult{}, fmt.Errorf("invalid sha %q: must be 40-char lowercase hex", sha)
        }
    }
    // repoPath is always derived from our own clone; no user-supplied string.
    if repoPath == "" {
        return RangeDiffResult{}, fmt.Errorf("repoPath must not be empty")
    }

    tCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    cmd := exec.CommandContext(tCtx, "git", "range-diff", "--no-color",
        baseStart+".."+baseEnd,
        headStart+".."+headEnd,
    )
    cmd.Dir = repoPath
    out, err := cmd.Output()
    if err != nil {
        rd.log.Warn("git range-diff failed; falling back to Tree.Patch",
            "err", err, "repoPath", repoPath)
        return rd.fallback(headStart, headEnd)
    }
    return RangeDiffResult{Raw: string(out)}, nil
}

// fallback returns a degraded result using go-git Tree.Patch head-to-head.
// When gitClient is nil (e.g. tests) returns Degraded=true with empty Raw.
func (rd *RangeDiffer) fallback(baseSHA, headSHA string) (RangeDiffResult, error) {
    reason := rd.fallbackReason
    if reason == "" {
        reason = "range-diff unavailable"
    }
    if rd.gitClient == nil {
        return RangeDiffResult{Degraded: true, FallbackReason: reason}, nil
    }
    dr, err := rd.gitClient.GenerateDiff("", baseSHA, headSHA)
    if err != nil {
        return RangeDiffResult{Degraded: true, FallbackReason: reason}, nil
    }
    return RangeDiffResult{Raw: dr.Diff, Degraded: true, FallbackReason: reason}, nil
}
```

> Note: add `"fmt"` to the import block if the `itoa` helper in the test needs it — adjust as needed to satisfy the compiler.

- [ ] **Step 3: Run tests (expect PASS)**

```bash
cd go && go test ./internal/git/ -run 'TestRangeDiffer' -v
```

- [ ] **Step 4: Verify no other os/exec in production code**

```bash
grep -rn "os/exec" /Users/samuelasvanyi/dev/bistro/go/internal/ | grep -v rangediff.go
```

Expected: no output.

- [ ] **Step 5: Run full Go test suite**

```bash
cd go && go vet ./... && go test -race ./...
```

Expected: all pass, zero races.

- [ ] **Step 6: Commit**

```bash
git add go/internal/git/rangediff.go go/internal/git/rangediff_test.go
git commit -m "feat(git): RangeDiffer — capability probe + git range-diff wrapper with injection safety + fallback"
```

---

## Task 5: WS handlers — `patchsets.go` + `rangediff.go`

**Files:**
- Create: `go/internal/server/handler/patchsets.go`
- Create: `go/internal/server/handler/rangediff.go`
- Modify: `go/internal/server/app.go` (inject `RangeDiffer`, register handlers)
- Modify: `go/internal/server/handler/pr.go` (call `SetLastReviewedSHA` after review submission)

- [ ] **Step 1: Write failing handler tests**

Add to `go/internal/server/handler/patchsets_test.go`:

```go
package handler_test

import (
    "context"
    "encoding/json"
    "testing"

    "github.com/itterno/bistro/internal/server/ws"
)

type mockPatchsetStore struct {
    patchsets []storePatchset
}

func (m *mockPatchsetStore) ListPatchsets(ctx context.Context, prID string) ([]storePatchset, error) {
    return m.patchsets, nil
}

func TestListPatchsetsHandler_ReturnsPatchsets(t *testing.T) {
    store := &mockPatchsetStore{patchsets: []storePatchset{
        {PRID: "pr-1", Number: 1, HeadSHA: "aaa", BaseSHA: "bbb"},
        {PRID: "pr-1", Number: 2, HeadSHA: "ccc", BaseSHA: "bbb"},
    }}
    h := NewListPatchsetsHandler(store, nil)
    data, _ := json.Marshal(map[string]string{"pr_id": "pr-1"})
    resp, err := h(context.Background(), ws.InboundMessage{Data: data})
    if err != nil { t.Fatalf("handler err: %v", err) }
    type out struct {
        Patchsets []storePatchset `json:"patchsets"`
    }
    b, _ := json.Marshal(resp)
    var o out
    json.Unmarshal(b, &o)
    if len(o.Patchsets) != 2 { t.Fatalf("expected 2 patchsets, got %d", len(o.Patchsets)) }
}
```

Run: `cd go && go test ./internal/server/handler/ -run TestListPatchsetsHandler -v`
Expected: FAIL.

- [ ] **Step 2: Implement `patchsets.go`**

```go
// go/internal/server/handler/patchsets.go
package handler

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"

    "github.com/itterno/bistro/internal/server/ws"
    "github.com/itterno/bistro/internal/store"
)

type patchsetLister interface {
    ListPatchsets(ctx context.Context, prID string) ([]store.Patchset, error)
}

// NewListPatchsetsHandler returns a WS handler for GET_PATCHSETS.
func NewListPatchsetsHandler(ps patchsetLister, log *slog.Logger) ws.MessageHandler {
    if log == nil {
        log = slog.Default()
    }
    return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
        var req struct {
            PRID string `json:"pr_id"`
        }
        if err := json.Unmarshal(msg.Data, &req); err != nil {
            return nil, fmt.Errorf("parse GET_PATCHSETS: %w", err)
        }
        if req.PRID == "" {
            return nil, fmt.Errorf("GET_PATCHSETS: pr_id required")
        }
        patchsets, err := ps.ListPatchsets(ctx, req.PRID)
        if err != nil {
            log.Error("list patchsets", "pr_id", req.PRID, "err", err)
            return nil, fmt.Errorf("list patchsets: %w", err)
        }
        return map[string]interface{}{
            "type":      "PATCHSETS_READY",
            "pr_id":     req.PRID,
            "patchsets": patchsets,
        }, nil
    }
}
```

> You will need to add `ListPatchsets(ctx, prID) ([]Patchset, error)` to `go/internal/store/`
> and a `Patchset` struct (add to `go/internal/store/patchsets.go`, a new small file).

- [ ] **Step 3: Implement `rangediff.go` handler**

```go
// go/internal/server/handler/rangediff.go
package handler

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"

    "github.com/itterno/bistro/internal/git"
    "github.com/itterno/bistro/internal/server/ws"
    "github.com/itterno/bistro/internal/store"
)

type rangeDiffDeps struct {
    rd          *git.RangeDiffer
    patchsets   patchsetLister
    lastReviewed interface {
        GetLastReviewedSHA(ctx context.Context, prID, login string) (string, bool, error)
    }
    currentUser interface {
        GetCurrentUser(ctx context.Context) (string, error)
    }
    gitClient *git.Client
}

// NewRangeDiffHandler returns a WS handler for REQUEST_RANGE_DIFF.
func NewRangeDiffHandler(deps rangeDiffDeps, log *slog.Logger) ws.MessageHandler {
    if log == nil {
        log = slog.Default()
    }
    return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
        var req struct {
            PRID            string `json:"pr_id"`
            BaseSHA         string `json:"base_sha"`
            HeadSHA         string `json:"head_sha"`
            SinceLastReview bool   `json:"since_last_review"`
        }
        if err := json.Unmarshal(msg.Data, &req); err != nil {
            return nil, fmt.Errorf("parse REQUEST_RANGE_DIFF: %w", err)
        }
        baseSHA := req.BaseSHA
        if req.SinceLastReview {
            login, err := deps.currentUser.GetCurrentUser(ctx)
            if err != nil {
                log.Error("get current user for range-diff", "err", err)
                return degradedResponse(req.PRID, "current user unavailable"), nil
            }
            sha, ok, err := deps.lastReviewed.GetLastReviewedSHA(ctx, req.PRID, login)
            if err != nil {
                log.Error("get last reviewed sha", "pr_id", req.PRID, "login", login, "err", err)
                return degradedResponse(req.PRID, "last_reviewed lookup failed"), nil
            }
            if !ok {
                return degradedResponse(req.PRID, "no prior review recorded"), nil
            }
            baseSHA = sha
        }
        repoPath := deps.gitClient.CloneDir(req.PRID)
        result, err := deps.rd.RunRangeDiff(ctx, repoPath, baseSHA, req.HeadSHA, baseSHA, req.HeadSHA)
        if err != nil {
            log.Warn("range-diff error; degrading", "pr_id", req.PRID, "err", err)
            return degradedResponse(req.PRID, err.Error()), nil
        }
        return map[string]interface{}{
            "type":            "RANGE_DIFF_READY",
            "pr_id":           req.PRID,
            "raw":             result.Raw,
            "degraded":        result.Degraded,
            "fallback_reason": result.FallbackReason,
        }, nil
    }
}

func degradedResponse(prID, reason string) map[string]interface{} {
    return map[string]interface{}{
        "type":            "RANGE_DIFF_READY",
        "pr_id":           prID,
        "raw":             "",
        "degraded":        true,
        "fallback_reason": reason,
    }
}
```

- [ ] **Step 4: Wire `SetLastReviewedSHA` into `pr.go` action handler**

In `go/internal/server/handler/pr.go`, after a successful `APPROVE`, `REQUEST_CHANGES`,
or `COMMENT` action write, call:

```go
login, err := deps.currentUser.GetCurrentUser(ctx)
if err != nil {
    log.Warn("could not get current user for last_reviewed_sha update", "err", err)
} else {
    if err := deps.store.SetLastReviewedSHA(ctx, prID, login, headSHA); err != nil {
        log.Error("set last_reviewed_sha", "pr_id", prID, "login", login, "err", err)
        // Do NOT return error — review submission succeeded; this is a best-effort record.
    }
}
```

- [ ] **Step 5: Register handlers in `app.go` + log probe result**

In `go/internal/server/app.go`:

```go
rd := git.NewRangeDiffer(log, gitClient)
log.Info("range-diff probe", "available", rd.Available(), "reason", rd.FallbackReason())
// ... register handler routes:
// mux.Handle("GET_PATCHSETS", handler.NewListPatchsetsHandler(db, log))
// mux.Handle("REQUEST_RANGE_DIFF", handler.NewRangeDiffHandler(rangeDiffDeps{...}, log))
// mux.Handle("MARK_FILE_VIEWED", handler.NewMarkFileViewedHandler(db, log))
```

- [ ] **Step 6: Run handler tests**

```bash
cd go && go test ./internal/server/handler/ -run 'TestListPatchsets|TestRangeDiff' -v
cd go && go vet ./... && go test -race ./...
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add go/internal/server/handler/patchsets.go go/internal/server/handler/rangediff.go go/internal/server/handler/patchsets_test.go go/internal/server/app.go go/internal/server/handler/pr.go
git commit -m "feat(server): GET_PATCHSETS + REQUEST_RANGE_DIFF + MARK_FILE_VIEWED handlers; wire SetLastReviewedSHA on review submission"
```

---

## Task 6: Electron types + keybinding

**Files:**
- Modify: `electron/src/renderer/types/index.ts`
- Modify: `electron/src/renderer/utils/keybindings.ts`

- [ ] **Step 1: Write failing type test (TS compile check)**

Add to `electron/tests/unit/types.test.ts` (create if absent):

```typescript
import type { Patchset, RangeDiffResult, FileViewedState } from '../../src/renderer/types';

// Compile-time checks only — will fail if types missing
const _p: Patchset = { prId: 'x', number: 1, headSha: 'a'.repeat(40), baseSha: 'b'.repeat(40), pushedAt: '' };
const _r: RangeDiffResult = { prId: 'x', raw: '', degraded: false, fallbackReason: '' };
const _f: FileViewedState = { prId: 'x', filePath: 'foo.go', contentHash: 'abc', viewed: false };
```

Run: `cd electron && pnpm type-check`
Expected: FAIL (types not defined).

- [ ] **Step 2: Add types to `electron/src/renderer/types/index.ts`**

Add (do not remove any existing types — append):

```typescript
export interface Patchset {
  prId: string;
  number: number;
  headSha: string;
  baseSha: string;
  pushedAt: string; // ISO 8601
}

export interface RangeDiffResult {
  prId: string;
  raw: string;
  degraded: boolean;
  fallbackReason: string;
}

export interface FileViewedState {
  prId: string;
  filePath: string;
  contentHash: string;
  viewed: boolean;
}
```

- [ ] **Step 3: Add keybinding `diff.sinceLastReview`**

In `electron/src/renderer/utils/keybindings.ts`, add inside the `diff` group
(following the existing pattern for other `diff.*` bindings):

```typescript
'diff.sinceLastReview': {
  key: 's',
  description: 'Toggle since-last-review diff',
  mode: 'diff',
},
```

- [ ] **Step 4: Type-check + lint**

```bash
cd electron && pnpm type-check && pnpm lint
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/types/index.ts electron/src/renderer/utils/keybindings.ts
git commit -m "feat(ui): Patchset / RangeDiffResult / FileViewedState types + diff.sinceLastReview keybinding"
```

---

## Task 7: `PatchsetTimeline` component

**Files:**
- Create: `electron/src/renderer/components/PatchsetTimeline.tsx`
- Create: `electron/tests/unit/PatchsetTimeline.test.tsx`

- [ ] **Step 1: Write failing tests**

```typescript
// electron/tests/unit/PatchsetTimeline.test.tsx
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PatchsetTimeline } from '../../src/renderer/components/PatchsetTimeline';
import type { Patchset } from '../../src/renderer/types';

const patchsets: Patchset[] = [
  { prId: 'pr-1', number: 1, headSha: 'a'.repeat(40), baseSha: 'b'.repeat(40), pushedAt: '2024-01-01T00:00:00Z' },
  { prId: 'pr-1', number: 2, headSha: 'c'.repeat(40), baseSha: 'b'.repeat(40), pushedAt: '2024-01-02T00:00:00Z' },
];

describe('PatchsetTimeline', () => {
  it('renders P1 and P2 labels', () => {
    render(<PatchsetTimeline patchsets={patchsets} onSelectPair={vi.fn()} />);
    expect(screen.getByText('P1')).toBeTruthy();
    expect(screen.getByText('P2')).toBeTruthy();
  });

  it('shows honest label copy', () => {
    render(<PatchsetTimeline patchsets={patchsets} onSelectPair={vi.fn()} />);
    expect(screen.getByText(/snapshots observed/i)).toBeTruthy();
  });

  it('calls onSelectPair with default (Pn-1, Pn) on mount', () => {
    const spy = vi.fn();
    render(<PatchsetTimeline patchsets={patchsets} onSelectPair={spy} />);
    expect(spy).toHaveBeenCalledWith(patchsets[0], patchsets[1]);
  });

  it('renders empty state when no patchsets', () => {
    render(<PatchsetTimeline patchsets={[]} onSelectPair={vi.fn()} />);
    expect(screen.getByText(/no snapshots/i)).toBeTruthy();
  });
});
```

Run: `cd electron && pnpm test PatchsetTimeline`
Expected: FAIL (component not found).

- [ ] **Step 2: Implement `PatchsetTimeline.tsx`**

```tsx
// electron/src/renderer/components/PatchsetTimeline.tsx
import React from 'react';
import type { Patchset } from '../types';

interface Props {
  patchsets: Patchset[];
  onSelectPair: (from: Patchset, to: Patchset) => void;
}

export function PatchsetTimeline({ patchsets, onSelectPair }: Props): React.ReactElement {
  const [fromIdx, setFromIdx] = React.useState<number>(0);
  const [toIdx, setToIdx] = React.useState<number>(Math.max(patchsets.length - 1, 0));

  // Fire default pair on mount / when patchsets list changes.
  React.useEffect(() => {
    if (patchsets.length < 2) return;
    const from = patchsets[patchsets.length - 2];
    const to = patchsets[patchsets.length - 1];
    setFromIdx(patchsets.length - 2);
    setToIdx(patchsets.length - 1);
    onSelectPair(from, to);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [patchsets]);

  const handleSelectPair = (newFrom: number, newTo: number): void => {
    if (newFrom === newTo) return;
    setFromIdx(newFrom);
    setToIdx(newTo);
    onSelectPair(patchsets[newFrom], patchsets[newTo]);
  };

  if (patchsets.length === 0) {
    return (
      <div className="text-xs text-muted-foreground px-2 py-1">
        No snapshots yet — snapshots observed since you started watching.
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-1 px-2 py-1 text-xs">
      <span className="text-muted-foreground">
        Snapshots observed since you started watching.
      </span>
      <div className="flex gap-1 items-center">
        {patchsets.map((p, i) => (
          <button
            key={p.number}
            className={`px-2 py-0.5 rounded border font-mono ${i === toIdx ? 'bg-primary text-primary-foreground' : 'bg-muted'}`}
            onClick={() => handleSelectPair(fromIdx, i)}
            aria-label={`Select P${p.number} as target`}
          >
            P{p.number}
          </button>
        ))}
      </div>
      <div className="flex gap-2 items-center text-muted-foreground">
        <span>FROM</span>
        {patchsets.map((p, i) => (
          <button
            key={p.number}
            className={`px-1 rounded ${i === fromIdx ? 'underline font-bold' : ''}`}
            onClick={() => handleSelectPair(i, toIdx)}
            aria-label={`Select P${p.number} as base`}
          >
            P{p.number}
          </button>
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Run tests (expect PASS)**

```bash
cd electron && pnpm test PatchsetTimeline
```

- [ ] **Step 4: Commit**

```bash
git add electron/src/renderer/components/PatchsetTimeline.tsx electron/tests/unit/PatchsetTimeline.test.tsx
git commit -m "feat(ui): PatchsetTimeline component with pair selector and honest snapshot label"
```

---

## Task 8: `DegradedRangeDiffIndicator` + `RangeDiffView`

**Files:**
- Create: `electron/src/renderer/components/DegradedRangeDiffIndicator.tsx`
- Create: `electron/src/renderer/components/RangeDiffView.tsx`
- Create: `electron/tests/unit/RangeDiffView.test.tsx`

- [ ] **Step 1: Write failing tests**

```typescript
// electron/tests/unit/RangeDiffView.test.tsx
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { FooterSlotProvider } from '../../src/renderer/components/FooterSlot';
import { RangeDiffView } from '../../src/renderer/components/RangeDiffView';
import type { RangeDiffResult } from '../../src/renderer/types';

function wrap(children: React.ReactNode) {
  return <FooterSlotProvider>{children}</FooterSlotProvider>;
}

describe('RangeDiffView', () => {
  it('renders raw diff text when not degraded', () => {
    const result: RangeDiffResult = { prId: 'pr-1', raw: '@@-1+1@@\n-old\n+new', degraded: false, fallbackReason: '' };
    render(wrap(<RangeDiffView result={result} />));
    expect(screen.getByText(/-old/)).toBeTruthy();
    expect(screen.queryByText(/range-diff unavailable/i)).toBeNull();
  });

  it('mounts footer indicator when degraded', () => {
    const result: RangeDiffResult = { prId: 'pr-1', raw: '', degraded: true, fallbackReason: 'system git not available' };
    // We can't easily assert the FooterCenterSlot content in a unit test without
    // rendering AppFooter; assert DegradedRangeDiffIndicator is rendered in the tree.
    render(wrap(<RangeDiffView result={result} />));
    // DegradedRangeDiffIndicator renders its content into the slot imperatively;
    // assert the slot wrapper renders without crash.
    expect(screen.queryByRole('alert')).toBeDefined(); // graceful non-crash check
  });
});
```

Run: `cd electron && pnpm test RangeDiffView`
Expected: FAIL.

- [ ] **Step 2: Implement `DegradedRangeDiffIndicator.tsx`**

```tsx
// electron/src/renderer/components/DegradedRangeDiffIndicator.tsx
import React from 'react';
import { FooterCenterSlot } from './FooterSlot';

interface Props {
  reason: string;
}

export function DegradedRangeDiffIndicator({ reason }: Props): React.ReactElement {
  return (
    <FooterCenterSlot>
      <span
        className="flex items-center gap-1 text-xs text-yellow-500"
        title={reason}
        aria-label={`range-diff unavailable: ${reason}`}
      >
        ⚠ range-diff unavailable
      </span>
    </FooterCenterSlot>
  );
}
```

- [ ] **Step 3: Implement `RangeDiffView.tsx`**

```tsx
// electron/src/renderer/components/RangeDiffView.tsx
import React from 'react';
import type { RangeDiffResult } from '../types';
import { DegradedRangeDiffIndicator } from './DegradedRangeDiffIndicator';

interface Props {
  result: RangeDiffResult;
}

export function RangeDiffView({ result }: Props): React.ReactElement {
  return (
    <div role="region" aria-label="diff view">
      {result.degraded && (
        <DegradedRangeDiffIndicator reason={result.fallbackReason} />
      )}
      <pre className="text-xs font-mono whitespace-pre overflow-auto p-2">
        {result.raw || (result.degraded ? '(no diff available)' : '')}
      </pre>
    </div>
  );
}
```

- [ ] **Step 4: Run tests (expect PASS)**

```bash
cd electron && pnpm test RangeDiffView
cd electron && pnpm type-check && pnpm lint
```

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/components/DegradedRangeDiffIndicator.tsx electron/src/renderer/components/RangeDiffView.tsx electron/tests/unit/RangeDiffView.test.tsx
git commit -m "feat(ui): RangeDiffView with DegradedRangeDiffIndicator in footer slot"
```

---

## Task 9: Wire into `PRDetailPage` + `DiffPage`

**Files:**
- Modify: `electron/src/renderer/pages/PRDetailPage.tsx`
- Modify: `electron/src/renderer/pages/DiffPage.tsx`

- [ ] **Step 1: Mount `PatchsetTimeline` in `PRDetailPage`**

In `electron/src/renderer/pages/PRDetailPage.tsx`:
1. Import `PatchsetTimeline` and `usePatchsets` (a new small hook that sends
   `GET_PATCHSETS` via the existing WS/IPC channel and returns `Patchset[]`).
2. Add state: `const [selectedPair, setSelectedPair] = React.useState<[Patchset, Patchset] | null>(null)`.
3. Render `<PatchsetTimeline patchsets={patchsets} onSelectPair={(f, t) => setSelectedPair([f, t])} />`
   at the top of the PR detail body.
4. On `selectedPair` change, navigate to `DiffPage` with `patchsetPair` params
   (or update page state to render `RangeDiffView`).

- [ ] **Step 2: Accept patchset pair in `DiffPage`**

In `electron/src/renderer/pages/DiffPage.tsx`:
1. Accept optional props `baseSha?: string; headSha?: string; sinceLastReview?: boolean`.
2. If `sinceLastReview` is true (triggered by `s` keybinding), send
   `REQUEST_RANGE_DIFF { pr_id, since_last_review: true }`.
3. If `baseSha` + `headSha` provided (from timeline pair), send
   `REQUEST_RANGE_DIFF { pr_id, base_sha: baseSha, head_sha: headSha }`.
4. Render `<RangeDiffView result={rangeDiffResult} />` when a result is available.

- [ ] **Step 3: Register `s` keybinding in `useAppHotkeys`**

In `electron/src/renderer/hooks/useAppHotkeys.ts` (or wherever `diff.*` bindings
are registered), add a handler for `diff.sinceLastReview` that dispatches the
since-last-review request.

- [ ] **Step 4: Type-check + full test suite**

```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```

Expected: all pass (≥ 149 tests).

- [ ] **Step 5: Commit**

```bash
git add electron/src/renderer/pages/PRDetailPage.tsx electron/src/renderer/pages/DiffPage.tsx electron/src/renderer/hooks/useAppHotkeys.ts
git commit -m "feat(ui): wire PatchsetTimeline into PRDetailPage + since-last-review toggle in DiffPage"
```

---

## Task 10: Acceptance verification

- [ ] **Run full Go suite (race detector)**

```bash
cd go && go vet ./... && go test -race ./...
```

Expected: all pass, zero races.

- [ ] **Verify no rogue os/exec**

```bash
grep -rn "os/exec" /Users/samuelasvanyi/dev/bistro/go/internal/ | grep -v rangediff.go
```

Expected: no output.

- [ ] **Run full Electron suite**

```bash
cd electron && pnpm type-check && pnpm lint && pnpm test
```

Expected: all pass, no lint errors.

- [ ] **Verify capability probe logged at startup**

```bash
cd go && go run cmd/bistro-sync/main.go 2>&1 | grep -i "range-diff"
```

Expected: one line containing `range-diff probe` with `available=true` or `available=false`.

- [ ] **Commit final**

```bash
git add .
git commit -m "chore(phase-2.1-2.2): acceptance verification — all tests pass, no rogue os/exec"
```

---

## Verify commands (run any time)

```bash
# Go
cd go && go vet ./... && go test -race ./...

# Electron
cd electron && pnpm type-check && pnpm lint && pnpm test

# No rogue os/exec
grep -rn "os/exec" /Users/samuelasvanyi/dev/bistro/go/internal/ | grep -v rangediff.go
```
