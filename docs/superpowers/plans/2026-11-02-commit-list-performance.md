# Commit List Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: subagent-driven-development task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** With a warm stats cache, make opening the commit list and stepping between commits take tens of milliseconds instead of seconds; the remaining diffing happens once per new commit, in sync.

**Architecture:** Per-commit list statistics (+/-, file count, fingerprint) move into a SQLite cache keyed by `(pr_id, sha)`, filled by PR sync while it already holds the clone and topped up on demand by `Commits.List`. `List` walks the range without diffing and reads the cache. The renderer loads the list once per PR, shares in-flight requests, and stops blocking the commit page on the list.

**Tech Stack:** Go 1.26.1, go-git v5.19.3, SQLite + goose + sqlc v1.31.1, Wails v3 bindings, React 19 + TanStack, Vitest.

**Spec:** `docs/superpowers/specs/2026-11-02-commit-by-commit-review-design.md` — implement **Amendment 1** only; the rest of the spec is already shipped on this branch.

## Global Constraints

- Measured baseline (PR with 89 commits): `Get` 13 ms, range walk 10 ms, `List` 2.4 s. Targets (warm cache): a `List` costs the walk plus one query; stepping between commits renders after one `Get`, and the commit page's background list refresh starts only after that `Get` settles.
- Renderer list cache key: `${viewerLogin}\u0000${prId}` (`appStore.state.viewerLogin`), captured when the request starts. Only `available: true` lists are cached.
- Migration file: `internal/store/migrations/00039_add_commit_stats.sql`; table `commit_stats(pr_id, sha, additions, deletions, file_count, patch_id, algo_version)`, `PRIMARY KEY (pr_id, sha)`, `FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE`.
- Store package: `internal/store/commitstats`, independent like `store/commitviewed`; it may import `store` / `store/db`, never the reverse.
- `algo_version` is the Go constant `diff.CommitStatsVersion = 1`; a row with another version counts as missing.
- Merges get no row and keep the viewed key `merge:<sha>`. The viewed-key scheme (`<PatchID>:<n>` + occurrences) is unchanged and must produce identical keys from cached patch IDs.
- A stats failure during sync is logged and never fails the PR sync or rolls back the PR row, patchset, or diff.
- `Get` keeps computing the single commit's patch on demand. Routes, keys, and the list layout are unchanged.
- Never hand-edit `internal/store/db/` or `frontend/bindings/`; regenerate with `wails3 task generate`.
- Frontend: `@thourum/fault` Result chains, never `try`/`catch`; requests and appStore writes live in `frontend/src/services/`; utility tests only, no React component tests.
- Debug timing logs (`commit range timing`, `service call timing`) stay, at debug level.
- Commit format `type: description`; implementers commit their task; never `--no-verify`, never push.

## File Structure

| File | Responsibility |
|---|---|
| `internal/git/diff/commits.go` | Range metadata reads, single-commit diff, repo open with kept descriptors |
| `internal/git/diff/commits_stats.go` (new) | `CommitStats`, `CommitStatsVersion`, `ComputeCommitStats` (locked), `CollectRangeStats` (caller holds lock) |
| `internal/store/migrations/00039_add_commit_stats.sql`, `internal/store/queries/commit_stats.sql` | Schema + sqlc queries |
| `internal/store/commitstats/store.go` (new) | `List`, `Upsert`, `ReplaceTx` |
| `internal/services/commits.go`, `commits_keys.go` | `List` / `SetViewed` read cache, compute missing; `Get` returns `is_merge` |
| `internal/engine/services.go` | Wiring |
| `internal/prsync/fetch.go`, `internal/prsync/writer/fetched_pr.go`, `internal/prsync/writer/commit_stats.go` (new), `writer.go` | Sync precompute + best-effort write |
| `frontend/src/services/commits.ts` | In-flight sharing, per-PR list cache |
| `frontend/src/pages/useCommitsData.ts`, `useCommitData.ts`, `CommitPage.tsx` | Load once per PR; don't block on list |
| `AGENTS.md` | Architecture notes |

---

### Task 1: Git layer — kept descriptors, metadata-only range, commit stats

**Files:**
- Modify: `internal/git/diff/commits.go`
- Create: `internal/git/diff/commits_stats.go`
- Modify: `internal/git/diff/commits_test.go`

**Interfaces:**
- Produces:
  - `type CommitStats struct { SHA string; Additions, Deletions, FileCount int; PatchID string }`
  - `const CommitStatsVersion = 1`
  - `func ListCommitRange(locks *repolock.Locks, req CommitRangeRequest) (CommitRange, error)` — metadata only; the `WithDiffs` field is **removed** from `CommitRangeRequest`, and `RangeCommit.Diff` is always nil from this function.
  - `func ComputeCommitStats(locks *repolock.Locks, req CommitRangeRequest, shas []string) ([]CommitStats, error)` — takes the repo lock; computes stats for the given non-merge SHAs (each must be a commit in the repo; callers pass SHAs from the range). Order of the result matches `shas`.
  - `func CollectRangeStats(repoPath, headSHA, mergeBaseSHA string, cached map[string]CommitStats) ([]CommitStats, error)` — **caller holds the repo lock** (used inside `WithPRClone`). Walks the capped range, returns stats for every non-merge commit in it (oldest first), reusing `cached[sha]` entries and computing the rest.
  - `GetRangeCommit` unchanged in signature.

- [ ] **Step 1: Open repositories with kept pack descriptors**

In `commits.go`, replace `gitlib.PlainOpen` in `cappedRange` with a helper that the caller closes. Clones are bare; test fixtures may be non-bare, so use `<path>/.git` when it exists:

```go
import (
	"os"
	"path/filepath"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// openRangeRepo keeps pack descriptors open for the read; without it go-git
// re-opens the packfile for every object (~30% of a range diff's CPU).
func openRangeRepo(path string) (*gitlib.Repository, func(), error) {
	dot := path
	if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && info.IsDir() {
		dot = filepath.Join(path, ".git")
	}
	storage := filesystem.NewStorageWithOptions(osfs.New(dot), cache.NewObjectLRUDefault(),
		filesystem.Options{KeepDescriptors: true})
	repo, err := gitlib.Open(storage, nil)
	if err != nil {
		_ = storage.Close()
		return nil, nil, fmt.Errorf("open repo %s: %w", path, err)
	}
	return repo, func() { _ = storage.Close() }, nil
}
```

Change `cappedRange(req)` to `cappedRange(repo *gitlib.Repository, headSHA, mergeBaseSHA string) ([]*object.Commit, bool, error)`; each caller opens with `openRangeRepo`, `defer closeRepo()`, then calls it. Commit objects must not be used after close, so all diffing happens inside the same function as the open.

- [ ] **Step 2: Make `ListCommitRange` metadata-only**

Remove `WithDiffs` from `CommitRangeRequest` and the diff loop from `ListCommitRange`. Keep the debug timing log but drop `with_diffs` / `diffed` attributes:

```go
func ListCommitRange(locks *repolock.Locks, req CommitRangeRequest) (CommitRange, error) {
	var out CommitRange
	started := time.Now()
	var lockWait time.Duration
	defer func() {
		slog.Debug("commit range timing", "op", "list", "lock_wait_ms", lockWait.Milliseconds(),
			"walk_ms", (time.Since(started) - lockWait).Milliseconds(), "commits", len(out.Commits))
	}()
	err := locks.WithRepo(repolock.Key(req.RepoURL), func() error {
		lockWait = time.Since(started)
		repo, closeRepo, err := openRangeRepo(req.RepoPath)
		if err != nil {
			return err
		}
		defer closeRepo()
		commits, truncated, err := cappedRange(repo, req.HeadSHA, req.MergeBaseSHA)
		if err != nil {
			return err
		}
		out.Truncated = truncated
		out.Commits = make([]RangeCommit, 0, len(commits))
		for _, c := range commits {
			out.Commits = append(out.Commits, toRangeCommit(c))
		}
		return nil
	})
	return out, err
}
```

`GetRangeCommit` opens via `openRangeRepo` the same way; its timing log stays.

- [ ] **Step 3: Add `commits_stats.go`**

```go
package diff

import (
	"fmt"
	"log/slog"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/itterno/bistro/internal/git/repolock"
)

// CommitStatsVersion invalidates cached stats when diff options or the
// fingerprint change; bump it with any such change.
const CommitStatsVersion = 1

// CommitStats is the cacheable list data of one non-merge commit. A SHA fixes
// its tree and first parent, so the stats never change for a given version.
type CommitStats struct {
	SHA       string
	Additions int
	Deletions int
	FileCount int
	PatchID   string
}

func statsOf(sha string, d CommitDiff) CommitStats {
	return CommitStats{SHA: sha, Additions: d.Additions, Deletions: d.Deletions, FileCount: len(d.Files), PatchID: d.PatchID}
}

// ComputeCommitStats diffs the given non-merge commits under the repo lock.
func ComputeCommitStats(locks *repolock.Locks, req CommitRangeRequest, shas []string) ([]CommitStats, error) {
	out := make([]CommitStats, 0, len(shas))
	started := time.Now()
	defer func() {
		slog.Debug("commit range timing", "op", "stats", "requested", len(shas),
			"computed", len(out), "duration_ms", time.Since(started).Milliseconds())
	}()
	err := locks.WithRepo(repolock.Key(req.RepoURL), func() error {
		repo, closeRepo, err := openRangeRepo(req.RepoPath)
		if err != nil {
			return err
		}
		defer closeRepo()
		for _, sha := range shas {
			c, err := repo.CommitObject(plumbing.NewHash(sha))
			if err != nil {
				return fmt.Errorf("load commit %s: %w", sha, err)
			}
			d, err := commitDiff(c)
			if err != nil {
				return err
			}
			out = append(out, statsOf(sha, d))
		}
		return nil
	})
	return out, err
}

// CollectRangeStats is for callers that already hold the repo lock (sync's
// WithPRClone callback); it must not take the lock itself.
func CollectRangeStats(repoPath, headSHA, mergeBaseSHA string, cached map[string]CommitStats) ([]CommitStats, error) {
	repo, closeRepo, err := openRangeRepo(repoPath)
	if err != nil {
		return nil, err
	}
	defer closeRepo()
	return collectRangeStats(repo, headSHA, mergeBaseSHA, cached)
}

func collectRangeStats(repo *gitlib.Repository, headSHA, mergeBaseSHA string, cached map[string]CommitStats) ([]CommitStats, error) {
	commits, _, err := cappedRange(repo, headSHA, mergeBaseSHA)
	if err != nil {
		return nil, err
	}
	out := make([]CommitStats, 0, len(commits))
	for _, c := range commits {
		if len(c.ParentHashes) > 1 {
			continue
		}
		sha := c.Hash.String()
		if stats, ok := cached[sha]; ok {
			out = append(out, stats)
			continue
		}
		d, err := commitDiff(c)
		if err != nil {
			return nil, err
		}
		out = append(out, statsOf(sha, d))
	}
	return out, nil
}
```

Remove the now-unused `CommitDiff` fields only if nothing uses them; `GetRangeCommit` still returns `Diff`.

- [ ] **Step 4: Update tests in `commits_test.go`**

Existing tests that set `WithDiffs: true` and read `Diff` from `ListCommitRange` move to `ComputeCommitStats` / `CollectRangeStats`. Add these focused tests using the existing `gittest` fixtures in that file:

1. `TestCollectRangeStats_ReusesCachedAndSkipsMerges` — build a range with two ordinary commits and one merge (the existing merge fixture). Pass `cached` holding a sentinel `CommitStats{SHA: <first>, PatchID: "sentinel"}`. Assert: the result has exactly the two non-merge SHAs, oldest first; the first entry's `PatchID == "sentinel"` (reused, not recomputed); the second entry's `PatchID` equals the one `ComputeCommitStats` returns for that SHA.
2. `TestComputeCommitStats_MatchesGetRangeCommit` — for one commit, `ComputeCommitStats` returns `Additions`, `Deletions`, `FileCount`, `PatchID` equal to `GetRangeCommit`'s `Diff.Additions`, `Diff.Deletions`, `len(Diff.Files)`, `Diff.PatchID`. This pins that cached stats and on-demand diffs agree.
3. Keep the existing range tests (excludes base ancestors, order, merge flag, truncation at 250) passing against metadata-only `ListCommitRange`.

- [ ] **Step 5: Verify**

Run: `go vet ./internal/git/... && go test -race ./internal/git/diff/`
Expected: `ok`. (`internal/services` and `internal/engine` will not compile until Task 3 — do not fix them here beyond what is needed for `go vet ./internal/git/...`.)

Because removing `WithDiffs` breaks `internal/services` compilation, make the **minimal** compile fix in this task so the tree builds: in `internal/services/commits.go`, delete the two `rangeReq.WithDiffs = true` lines. List stats and viewed keys will be wrong until Task 3; that is expected, and Task 3 replaces this code. Run `go build ./... ` and expect success; `go test ./internal/services/` may fail on stats/viewed tests — note which in the report.

- [ ] **Step 6: Commit**

```bash
git add internal/git/diff internal/services/commits.go
git commit -m "perf: keep pack descriptors and split commit stats from range listing"
```

---

### Task 2: `commit_stats` table and store

**Files:**
- Create: `internal/store/migrations/00039_add_commit_stats.sql`
- Create: `internal/store/queries/commit_stats.sql`
- Generated: `internal/store/db/*` (sqlc)
- Create: `internal/store/commitstats/store.go`, `internal/store/commitstats/store_test.go`

**Interfaces:**
- Produces (package `commitstats`):
  - `type Stats struct { SHA string; Additions, Deletions, FileCount int; PatchID string }`
  - `func New(db *sql.DB) *Store`
  - `func (s *Store) List(ctx context.Context, prID string, version int) (map[string]Stats, error)` — rows with `algo_version = version` only; non-nil empty map when none.
  - `func (s *Store) Upsert(ctx context.Context, prID string, version int, stats []Stats) error`
  - `func ReplaceTx(ctx context.Context, tx *sql.Tx, prID string, version int, stats []Stats) error` — deletes all of the PR's rows, then inserts `stats`.

- [ ] **Step 1: Migration**

```sql
-- +goose Up
-- Migration: 00039_add_commit_stats
-- Description: Cached list statistics for commits in a PR's range. A commit
-- SHA fixes its diff, so rows are only replaced by sync or invalidated by a
-- changed algo_version (diff options or fingerprint changed).
CREATE TABLE IF NOT EXISTS commit_stats (
    pr_id        TEXT    NOT NULL,
    sha          TEXT    NOT NULL,
    additions    INTEGER NOT NULL,
    deletions    INTEGER NOT NULL,
    file_count   INTEGER NOT NULL,
    patch_id     TEXT    NOT NULL,
    algo_version INTEGER NOT NULL,
    PRIMARY KEY (pr_id, sha),
    FOREIGN KEY (pr_id) REFERENCES prs(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS commit_stats;
```

- [ ] **Step 2: Queries**

```sql
-- name: ListCommitStats :many
SELECT sha, additions, deletions, file_count, patch_id
FROM commit_stats
WHERE pr_id = sqlc.arg(pr_id) AND algo_version = sqlc.arg(algo_version);

-- name: UpsertCommitStats :exec
INSERT INTO commit_stats (pr_id, sha, additions, deletions, file_count, patch_id, algo_version)
VALUES (sqlc.arg(pr_id), sqlc.arg(sha), sqlc.arg(additions), sqlc.arg(deletions), sqlc.arg(file_count), sqlc.arg(patch_id), sqlc.arg(algo_version))
ON CONFLICT(pr_id, sha) DO UPDATE SET
    additions = excluded.additions,
    deletions = excluded.deletions,
    file_count = excluded.file_count,
    patch_id = excluded.patch_id,
    algo_version = excluded.algo_version;

-- name: DeleteCommitStatsForPR :exec
DELETE FROM commit_stats WHERE pr_id = sqlc.arg(pr_id);
```

Run `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` from the repo root.

- [ ] **Step 3: Store**

Model it on `internal/store/commitviewed/store.go` and `internal/store/prdiff/prdiff.go` (`ReplaceTx` uses `storedb.New(tx)`). `Upsert` runs inside one transaction on `s.db` so a 250-row upsert is one commit:

```go
// Package commitstats caches per-commit list statistics for PR ranges.
package commitstats

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) List(ctx context.Context, prID string, version int) (map[string]Stats, error) {
	rows, err := storedb.New(s.db).ListCommitStats(ctx, storedb.ListCommitStatsParams{PrID: prID, AlgoVersion: int64(version)})
	if err != nil {
		return nil, fmt.Errorf("list commit stats (%s): %w", prID, err)
	}
	out := make(map[string]Stats, len(rows))
	for _, r := range rows {
		out[r.Sha] = Stats{SHA: r.Sha, Additions: int(r.Additions), Deletions: int(r.Deletions), FileCount: int(r.FileCount), PatchID: r.PatchID}
	}
	return out, nil
}

func (s *Store) Upsert(ctx context.Context, prID string, version int, stats []Stats) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin commit stats upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := upsertAll(ctx, storedb.New(tx), prID, version, stats); err != nil {
		return err
	}
	return tx.Commit()
}

func ReplaceTx(ctx context.Context, tx *sql.Tx, prID string, version int, stats []Stats) error {
	q := storedb.New(tx)
	if err := q.DeleteCommitStatsForPR(ctx, prID); err != nil {
		return fmt.Errorf("clear commit stats for %s: %w", prID, err)
	}
	return upsertAll(ctx, q, prID, version, stats)
}
```

`upsertAll` loops `UpsertCommitStats`. Use the generated field names sqlc actually emits (check `internal/store/db/commit_stats.sql.go`).

- [ ] **Step 4: Test**

One test, `TestCommitStats_ListFiltersVersionAndReplacePrunes`, against `storetest.Open(t)` with a PR row inserted the same way as `commitviewed/store_test.go`'s `insertPR`:
1. `Upsert(pr, 1, [a, b])`, then `Upsert(pr, 2, [c])`. `List(pr, 1)` returns exactly `{a, b}` with their fields; `List(pr, 2)` returns `{c}`.
2. In a transaction, `ReplaceTx(pr, 1, [b'])` (b with new values), commit. `List(pr, 1)` returns exactly `{b'}` — `a` pruned; `List(pr, 2)` is empty (Replace clears all versions).
3. `List` for a PR with no rows returns a non-nil empty map.

- [ ] **Step 5: Docs**

In `AGENTS.md`, add `commitstats` to the independent SQLite stores list (`currentuser`, `commitstats`, `commitviewed`, …).

- [ ] **Step 6: Verify and commit**

Run: `go vet ./internal/store/... && go test -race ./internal/store/...`
Expected: `ok` for every package.

```bash
git add internal/store AGENTS.md
git commit -m "feat: add commit_stats cache table and store"
```

---

### Task 3: `Commits` service reads the cache

**Files:**
- Modify: `internal/services/commits.go`, `internal/services/commits_keys.go`, `internal/services/commits_test.go`
- Modify: `internal/engine/services.go`
- Generated: `frontend/bindings/` (run `wails3 task generate`)

**Interfaces:**
- Consumes: Task 1 `diff.ListCommitRange` (metadata only), `diff.ComputeCommitStats`, `diff.CommitStats`, `diff.CommitStatsVersion`; Task 2 `commitstats.Store` (`List`, `Upsert`), `commitstats.Stats`.
- Produces:
  - New `Commits` fields: `Stats commitStatsStore` and `ComputeStats func(diff.CommitRangeRequest, []string) ([]diff.CommitStats, error)`.
  - `type commitStatsStore interface { List(ctx context.Context, prID string, version int) (map[string]commitstats.Stats, error); Upsert(ctx context.Context, prID string, version int, stats []commitstats.Stats) error }`
  - `CommitDiffResponse` gains `IsMerge bool \`json:"is_merge"\``.
  - `commitViewKeys(commits []diff.RangeCommit, patchIDs map[string]string) []commitViewKey` — a non-merge commit's fingerprint is `patchIDs[sha]`.

- [ ] **Step 1: Shared range-with-stats helper**

Add to `commits.go`:

```go
type statsRange struct {
	commits diff.CommitRange
	stats   map[string]commitstats.Stats // non-merge commits only
}

// rangeWithStats reads cached stats and computes only missing non-merge
// commits; a failed cache write is logged because the result is still valid.
func (s *Commits) rangeWithStats(ctx context.Context, prID string, req diff.CommitRangeRequest) (statsRange, error) {
	commits, err := s.Range(req)
	if err != nil {
		return statsRange{}, err
	}
	stats, err := s.Stats.List(ctx, prID, diff.CommitStatsVersion)
	if err != nil {
		return statsRange{}, err
	}
	var missing []string
	for _, c := range commits.Commits {
		if _, ok := stats[c.SHA]; !ok && !c.IsMerge {
			missing = append(missing, c.SHA)
		}
	}
	if len(missing) > 0 {
		computed, err := s.ComputeStats(req, missing)
		if err != nil {
			return statsRange{}, err
		}
		rows := make([]commitstats.Stats, len(computed))
		for i, c := range computed {
			rows[i] = commitstats.Stats{SHA: c.SHA, Additions: c.Additions, Deletions: c.Deletions, FileCount: c.FileCount, PatchID: c.PatchID}
			stats[c.SHA] = rows[i]
		}
		if err := s.Stats.Upsert(ctx, prID, diff.CommitStatsVersion, rows); err != nil {
			logging.FromContext(ctx).Warn("commits: caching stats failed", "error", err)
		}
	}
	return statsRange{commits: commits, stats: stats}, nil
}

func (r statsRange) patchIDs() map[string]string {
	ids := make(map[string]string, len(r.stats))
	for sha, st := range r.stats {
		ids[sha] = st.PatchID
	}
	return ids
}
```

Errors from `s.Range` / `s.ComputeStats` flow through the existing `rangeFailure` handling in `List` and the existing error mapping in `SetViewed` (they wrap git errors the same way). A `Stats.List` DB error returns as-is, like `Viewed.List` errors today.

- [ ] **Step 2: Use it in `List` and `SetViewed`**

In `List`, replace `rangeReq.WithDiffs = true; commits, err := s.Range(rangeReq)` with `sr, err := s.rangeWithStats(ctx, prID, rangeReq)`; the error branch stays `resp.Reason, err = rangeFailure(ctx, err)`. Build summaries from `sr.commits.Commits`, filling `Additions`, `Deletions`, `FileCount` from `sr.stats[commit.SHA]` for non-merges, and keys from `commitViewKeys(sr.commits.Commits, sr.patchIDs())`.

In `SetViewed`, do the same replacement; keep the existing error mapping (`contextError`, `ErrHeadUnavailable` → `CodeInvalid`, else internal). Compute `keys := commitViewKeys(...)` once before the loop (today it is recomputed inside the loop).

- [ ] **Step 3: `commitViewKeys` from patch IDs**

```go
// commitViewKeys returns keys aligned with commits; non-merge fingerprints
// come from patchIDs (cached or freshly computed stats).
func commitViewKeys(commits []diff.RangeCommit, patchIDs map[string]string) []commitViewKey {
	totals := map[string]int{}
	for _, commit := range commits {
		if !commit.IsMerge {
			totals[patchIDs[commit.SHA]]++
		}
	}
	seen := map[string]int{}
	keys := make([]commitViewKey, len(commits))
	for i, commit := range commits {
		if commit.IsMerge {
			keys[i] = commitViewKey{Key: "merge:" + commit.SHA, Occurrences: 1}
			continue
		}
		id := patchIDs[commit.SHA]
		keys[i] = commitViewKey{Key: fmt.Sprintf("%s:%d", id, seen[id]), Occurrences: totals[id]}
		seen[id]++
	}
	return keys
}
```

- [ ] **Step 4: `Get` returns `is_merge`**

In `Get`, after `resp.Available = true`, set `resp.IsMerge = commit.IsMerge`.

- [ ] **Step 5: Engine wiring**

In `internal/engine/services.go`'s `Commits:` literal add:

```go
Stats: commitstats.New(a.db.DB),
ComputeStats: func(req diff.CommitRangeRequest, shas []string) ([]diff.CommitStats, error) {
	return diff.ComputeCommitStats(a.repoLocks, req, shas)
},
```

Run `wails3 task generate` and confirm `frontend/bindings/` changes only add `is_merge` to `CommitDiffResponse`.

- [ ] **Step 6: Tests**

Update `newCommitsTestService` to set `Stats: commitstats.New(db)`. Existing tests that fed patch IDs through `Range`'s `Diff` now feed them through `ComputeStats` (a fake returning `diff.CommitStats` per SHA from a map). Keep all existing viewed-key behavior tests (rebase survives, duplicates separate, dropped duplicate resets) — they must pass unchanged in intent. Add:

1. `TestCommitsList_WarmCacheSkipsCompute` — pre-`Upsert` stats for every non-merge SHA the fake `Range` returns; set `ComputeStats` to `t.Fatal`. `List` returns the cached additions/deletions/file counts.
2. `TestCommitsList_PartialCacheComputesOnlyMissingAndStores` — cache one of two SHAs; `ComputeStats` records the SHAs it receives and must get exactly the missing one; after `List`, `Stats.List` holds both rows. A second `List` does not call `ComputeStats`.
3. `TestCommitsList_MergeNeverComputed` — a merge commit in the range is never passed to `ComputeStats` and reports zero stats.

- [ ] **Step 7: Verify and commit**

Run: `go vet ./... && go test -race ./internal/services/ ./internal/engine/ ./internal/git/...`
Expected: `ok`.

```bash
git add internal/services internal/engine frontend/bindings
git commit -m "perf: serve commit list stats from the commit_stats cache"
```

---

### Task 4: Sync precomputes commit stats

**Files:**
- Modify: `internal/prsync/fetch.go` (`generateDiff`)
- Modify: `internal/prsync/writer/fetched_pr.go`
- Create: `internal/prsync/writer/commit_stats.go`
- Modify: `internal/prsync/writer/writer.go` (`writeCriticalPRData`)
- Test: `internal/prsync/writer/commit_stats_test.go`, and one test in `internal/prsync` next to `fetch_mergebase_test.go`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: Task 1 `diff.CollectRangeStats`, `diff.CommitStatsVersion`; Task 2 `commitstats.New(db).List`, `commitstats.ReplaceTx`, `commitstats.Stats`.
- Produces: `FetchedPR.CommitStats []commitstats.Stats` and `FetchedPR.CommitStatsComputed bool`.

- [ ] **Step 1: Carry stats on `FetchedPR`**

```go
	// CommitStats are list statistics for every non-merge commit in the PR's
	// capped merge-base..head range. Written only when CommitStatsComputed;
	// the writer replaces the PR's commit_stats rows best-effort.
	CommitStats         []commitstats.Stats
	CommitStatsComputed bool
```

- [ ] **Step 2: Compute in `generateDiff`**

At the end of `generateDiff` (inside the `WithPRClone` callback, lock held, after `resolveDiffAnchors`), when `mergeBase != ""`:

```go
	o.collectCommitStats(ctx, result, clone, mergeBase, log)
```

```go
// collectCommitStats runs with the repo lock held; failures only cost a
// slower first commit-list open, so they never fail the PR fetch.
func (o *Orchestrator) collectCommitStats(ctx context.Context, result *writer.FetchedPR, clone *git.CloneResult, mergeBase string, log *slog.Logger) {
	if o.store == nil {
		return
	}
	started := time.Now()
	pr := result.PR
	cachedRows, err := commitstats.New(o.store.DB).List(ctx, pr.ID, gitdiff.CommitStatsVersion)
	if err != nil {
		log.Warn("commit stats: cache read failed", "error", err)
		return
	}
	cached := make(map[string]gitdiff.CommitStats, len(cachedRows))
	for sha, r := range cachedRows {
		cached[sha] = gitdiff.CommitStats{SHA: sha, Additions: r.Additions, Deletions: r.Deletions, FileCount: r.FileCount, PatchID: r.PatchID}
	}
	stats, err := gitdiff.CollectRangeStats(clone.LocalPath, pr.HeadSHA, mergeBase, cached)
	if err != nil {
		log.Warn("commit stats: collect failed", "error", err)
		return
	}
	result.CommitStats = make([]commitstats.Stats, len(stats))
	for i, s := range stats {
		result.CommitStats[i] = commitstats.Stats{SHA: s.SHA, Additions: s.Additions, Deletions: s.Deletions, FileCount: s.FileCount, PatchID: s.PatchID}
	}
	result.CommitStatsComputed = true
	logSubstep(log, "pr commit stats collected", time.Since(started).Milliseconds(),
		"commits", len(stats), "cached", len(cached))
}
```

`o.store.DB` is the `*sql.DB` on `*store.DB`; confirm the field name in `internal/store`. If the PR row does not exist yet (new PR), `List` returns an empty map — correct.

- [ ] **Step 3: Best-effort write inside the PR savepoint**

`internal/prsync/writer/commit_stats.go`:

```go
const commitStatsSavepoint = "commit_stats"

// writeCommitStats nests a savepoint so a failure discards only the stats
// rows, never the PR row, patchset, or diff written in the PR savepoint.
func (w *Writer) writeCommitStats(ctx context.Context, tx *sql.Tx, prID string, stats []commitstats.Stats) {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+commitStatsSavepoint); err != nil {
		w.log.Warn("commit stats: savepoint failed", "pr_id", prID, "error", err)
		return
	}
	if err := commitstats.ReplaceTx(ctx, tx, prID, gitdiff.CommitStatsVersion, stats); err != nil {
		w.log.Warn("commit stats: write failed", "pr_id", prID, "error", err)
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+commitStatsSavepoint); rbErr != nil {
			w.log.Warn("commit stats: rollback failed", "pr_id", prID, "error", rbErr)
		}
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+commitStatsSavepoint); err != nil {
		w.log.Warn("commit stats: release failed", "pr_id", prID, "error", err)
	}
}
```

Use the writer's existing logger field (check `Writer` in `writer.go`; it is constructed with `writer.New(o.log, o.store)`). In `writeCriticalPRData`, after the `writeDiffs` block:

```go
	if result.CommitStatsComputed {
		w.writeCommitStats(ctx, tx, pr.ID, result.CommitStats)
	}
```

- [ ] **Step 4: Tests**

1. `internal/prsync/writer/commit_stats_test.go` — `TestWriteSyncResults_CommitStatsReplacedAndPrunedWithPR`: write a `FetchedPR` for a new PR with `CommitStats [a, b]`, `CommitStatsComputed: true` through the writer's public batch write (follow how `diffs_test.go` drives the writer). `commitstats.List` returns `{a, b}`. Write again with `[b]` (force-push dropped `a`): `List` returns `{b}`. Write again with `CommitStatsComputed: false`: rows unchanged.
2. Same file — `TestWriteSyncResults_CommitStatsFailureKeepsPR`: make the stats write fail (e.g. a `CommitStats` entry whose `PatchID` violates no constraint won't fail — instead drop the table first: `DROP TABLE commit_stats` on the test DB, then write). Assert the PR row and its diffs were written and the write reported no error for the PR.
3. `internal/prsync` — `TestFetchDiff_CollectsCommitStatsForRange`: follow `fetch_mergebase_test.go`'s setup (real git fixture + orchestrator with a store) and assert that after `fetchPR` with diff enabled, `result.CommitStatsComputed` is true and `result.CommitStats` holds one entry per non-merge commit in merge-base..head, oldest first.

- [ ] **Step 5: Docs**

In `AGENTS.md`, extend the `diff` sentence about the commit range: "…`services.Commits` serves it with occurrence-aware patch-id viewed keys; per-commit list stats are cached in `store/commitstats`, filled by PR sync inside `WithPRClone` and topped up by `Commits.List`."

- [ ] **Step 6: Verify and commit**

Run: `go vet ./... && go test -race ./internal/prsync/... ./internal/store/...`
Expected: `ok`.

```bash
git add internal/prsync AGENTS.md
git commit -m "perf: precompute commit stats during PR sync"
```

---

### Task 5: Renderer — load once, share requests, don't block on the list

**Files:**
- Modify: `frontend/src/services/commits.ts`
- Modify: `frontend/src/pages/useCommitsData.ts`, `frontend/src/pages/useCommitData.ts`, `frontend/src/pages/CommitPage.tsx`
- Modify: `frontend/src/types` re-export if `CommitDiffResult` needs `is_merge` (it comes from bindings)
- Test: `frontend/src/services/commits.test.ts`

**Interfaces:**
- Consumes: Task 3 `CommitDiffResponse.is_merge`.
- Produces (in `services/commits.ts`):
  - `cachedCommitList(prId: string): CommitsResult | undefined` — the kept list for (current `viewerLogin`, `prId`).
  - `loadCommits(prId: string): ResultAsync<CommitsResult, Fault>` — always calls the engine, except that concurrent calls for the same (login, PR) key share one in-flight call; on success with `available: true` it stores the result under the key captured at start.
  - `cacheCommitList(login: string, prId: string, result: CommitsResult): void` — replaces the kept list for an explicit login (the one the hook scope started with), never the current one implicitly.
  - `cachedCommitList(prId)` / `loadCommits(prId)` read `appStore.state.viewerLogin` at call time.
  - `loadCommitDiff(prId, sha)` — concurrent calls for the same `prId:sha` share one in-flight call; results are not cached after settling.

- [ ] **Step 1: In-flight sharing and list cache**

```ts
const listCache = new Map<string, CommitsResult>();
const listInFlight = new Map<string, ResultAsync<CommitsResult, Fault>>();
const diffInFlight = new Map<string, ResultAsync<CommitDiffResult, Fault>>();

/** Viewed state is per account: never share a list across logins. */
function listKey(prId: string): string {
  return `${appStore.state.viewerLogin}\u0000${prId}`;
}

/** StrictMode double effects and remounts share one engine call. */
function shared<T>(
  inFlight: Map<string, ResultAsync<T, Fault>>,
  key: string,
  start: () => ResultAsync<T, Fault>,
): ResultAsync<T, Fault> {
  const pending = inFlight.get(key);
  if (pending !== undefined) return pending;
  const request = start();
  inFlight.set(key, request);
  void request.match(
    () => inFlight.delete(key),
    () => inFlight.delete(key),
  );
  return request;
}

export function cachedCommitList(prId: string): CommitsResult | undefined {
  return listCache.get(listKey(prId));
}

export function loadCommits(prId: string): ResultAsync<CommitsResult, Fault> {
  const key = listKey(prId); // fixed at start: a late response keeps its account
  return shared(listInFlight, key, () =>
    call(() => Commits.List({ pr_id: prId })).map((result) => {
      if (result.available) listCache.set(key, result);
      else listCache.delete(key);
      return result;
    }),
  );
}

export function cacheCommitList(
  login: string,
  prId: string,
  result: CommitsResult,
): void {
  listCache.set(`${login}\u0000${prId}`, result);
}

export function loadCommitDiff(
  prId: string,
  sha: string,
): ResultAsync<CommitDiffResult, Fault> {
  return shared(diffInFlight, `${prId}:${sha}`, () =>
    call(() => Commits.Get({ pr_id: prId, sha })),
  );
}
```

Check `@thourum/fault`'s installed API under `frontend/node_modules/@thourum/fault/` to confirm a `ResultAsync` can be matched by several callers (the shared request is). If it cannot, store the underlying `Promise<Result<T, Fault>>` and wrap it per caller with the library's constructor for an existing promise. Do not add `try`/`catch`.

- [ ] **Step 2: `useCommitsData` keeps the list across navigation**

- New signature: `useCommitsData(prId: string, selectedSha?: string, options?: { revalidateAfter?: unknown })`.
- Initial state: `result = cachedCommitList(prId) ?? null`, `loading = result === null`.
- The list page (`CommitsPage`, no `selectedSha`) fetches on mount and whenever `prId` changes: effect keyed on `[prId]`.
- The commit page (`useCommitData`) passes `revalidateAfter: diffState.diff ?? diffState.error ?? diffState.notInPr` — i.e. a value that changes when its `Get` settles. The fetch effect is keyed on `[prId, revalidateAfter]` and does nothing while `revalidateAfter` is `null`/`false` and a cached list exists (no fetch is needed to render). When no cached list exists, it fetches immediately (the header needs it; `Get` is not blocked because the page does not wait on the list).
- Never reset `result` to `null` on a `selectedSha` change; a new payload replaces it when it arrives. Keep `setCommitList(shas, selectedSha)` seeding in its own effect keyed on `[result, selectedSha]`.
- Keep the `useLayoutEffect` `clearCommitList()` keyed on `[prId]` only.
- The account is part of the hook's scope: read `viewerLogin` with `useStore(appStore, (s) => s.viewerLogin)`, add it to `scopeRef`'s scope object and to every effect key that today has `prId` (the layout `clearCommitList`, the fetch effect, and the state reset). A login change therefore behaves like a PR change: the displayed `result` resets to `cachedCommitList(prId)` for the new login (or `null`), pending marks are dropped, and a new fetch starts.
- Completions check the scope they started in (`scope.active && scopeRef.current === scope`, as `updateViewed` already does) before touching state, so a late response or rollback from the previous account never reaches the active view.
- `applyViewed` calls `cacheCommitList(scope.login, prId, next)` with the login captured in the toggle's scope, so a late rollback lands in its own account's entry.
- While a viewed toggle is pending for a SHA, a revalidated payload must not overwrite that SHA's optimistic `viewed` value: merge `viewed` from the current result for SHAs in `pendingRef.current`.

- [ ] **Step 3: Commit page doesn't wait for the list**

In `useCommitData`, return `loading: diffState.loading` (not `commits.loading || diffState.loading`) and wire `revalidateAfter` as above. In `CommitPage.tsx`, the merge branch tests `commit?.is_merge || diff?.is_merge` so a merge shows its message before the list arrives. The header already renders only when `commit !== null`.

- [ ] **Step 4: Test `services/commits.test.ts`**

Mock `@bindings/services` `Commits.List` / `Commits.Get` with deferred promises (follow how other `services/*.test.ts` files mock bindings). Tests:
1. Two concurrent `loadCommits('pr')` calls issue one `Commits.List`; both resolve to the same result, and `cachedCommitList('pr')` returns it.
2. Account isolation: with `viewerLogin = 'a'`, start `loadCommits('pr')`; switch `appStore` `viewerLogin` to `'b'` before resolving; resolve. `cachedCommitList('pr')` (as `b`) is `undefined`; switching back to `a` returns the result.
3. An `available: false` result is not kept (`cachedCommitList` is `undefined`), and replaces nothing usable: a previously kept available list for the same key is dropped.
4. Two concurrent `loadCommitDiff('pr', sha)` calls issue one `Commits.Get`; after settling, a third call issues a new one.
5. A failed `List` leaves the kept list unchanged.
6. `cacheCommitList('a', 'pr', x)` while `viewerLogin` is `'b'` is visible to `cachedCommitList('pr')` only after switching back to `'a'`.

Reset module state between tests (`vi.resetModules()` + dynamic import, or an exported `resetCommitCachesForTest` if the file's tests elsewhere use that pattern — prefer `vi.resetModules()`).

- [ ] **Step 5: Verify and commit**

Run: `cd frontend && pnpm type-check && pnpm lint && pnpm test`
Expected: all pass; no new lint warnings in changed files.

```bash
git add frontend/src
git commit -m "perf: load commit list once per PR and share in-flight commit requests"
```

---

## Manual verification (controller, after all tasks)

Run `BISTRO_LOG_LEVEL=debug wails3 dev 2>&1 | tee /tmp/bistro-commits-after.log` from the worktree, open the same PR, press `g c`, open a commit, step with `]` four times. In the log: one `Commits.List` per list entry; after sync has run, `List` lines with no `op=stats` line; per step, one `Commits.Get` whose `lock_wait_ms` is near 0, followed by at most one background `Commits.List` that starts after the `Get` finished. Also force-push or push a commit to a test PR while on a commit page, step once, and confirm the position count updates. Cold path: on a PR not yet synced with this build, note the `pr commit stats collected` duration in the sync log.
