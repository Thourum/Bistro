# Task 3: Migration 00035, store API, executor; delete the Go range-diff path

**Files:**
- Create: `go/internal/store/migrations/00035_add_patchset_fingerprints.sql`
- Modify: `go/internal/store/queries/patchsets.sql`, `last_reviewed.sql`, `diff_store.sql`
- Regenerate: `go/internal/store/db/` (sqlc — never hand-edit)
- Modify: `go/internal/store/patchsets.go`, `last_reviewed.go`, `diff_store.go`
- Modify: `go/internal/store/last_reviewed_test.go` (replace tests of removed functions)
- Modify: `go/internal/actions/executor.go`, `go/internal/actions/executor_test.go`
- Delete: `go/internal/git/rangediff.go`, `go/internal/git/rangediff_test.go`, `go/internal/server/handler/rangediff.go`
- Modify: `go/internal/server/handler/patchsets.go` (receive `patchsetByNumber`), `go/internal/server/handler/patchsets_test.go` (drop range-diff tests/mocks), `go/internal/server/app.go`

**Interfaces:**
- Consumes: nothing new.
- Produces (exact):

```go
// store/patchsets.go
type Patchset struct {
	PRID            string    `json:"pr_id"`
	Number          int       `json:"number"`
	HeadSHA         string    `json:"head_sha"`
	BaseSHA         string    `json:"base_sha"`
	MergeBaseSHA    string    `json:"merge_base_sha"`
	HasFingerprints bool      `json:"has_fingerprints"`
	PushedAt        time.Time `json:"pushed_at"`
}
func (db *DB) ListPatchsets(ctx context.Context, prID string) ([]Patchset, error)          // unchanged signature
func (db *DB) ListPatchsetFiles(ctx context.Context, prID string, number int) (map[string]string, error)
func (db *DB) CurrentPatchset(ctx context.Context, prID string) (Patchset, bool, error)

// store/last_reviewed.go
type ReviewSource string
const (
	ReviewSourceGitHubReview ReviewSource = "github_review"
	ReviewSourceBistroAction ReviewSource = "bistro_action"
	ReviewSourceAllViewed    ReviewSource = "all_viewed"
)
type ReviewEvent struct { PRID, Login, HeadSHA string; PatchsetNum int; ReviewedAt time.Time; Source ReviewSource }
type LastReviewedRecord struct { SHA string; PatchsetNum int; ReviewedAt time.Time; Source ReviewSource }
func RecordLastReviewed(ctx context.Context, q storedb.DBTX, event ReviewEvent) error
func (db *DB) RecordLastReviewed(ctx context.Context, event ReviewEvent) error
func (db *DB) GetLastReviewed(ctx context.Context, prID, login string) (LastReviewedRecord, bool, error)

// store/diff_store.go — store.DiffFile gains:
	PatchID string // git.PatchID of the full patch; "" for rows written before migration 035
```

Removed: `SetLastReviewedPatchset`, `GetLastReviewedPatchset`, `SetLastReviewedSHA`, `GetLastReviewedSHA` (and their queries), the `REQUEST_RANGE_DIFF` handler, `git.RangeDiffer`.

- [ ] **Step 1: Write the migration**

`go/internal/store/migrations/00035_add_patchset_fingerprints.sql` (check `migration_runner.go` / `00034` for how files are embedded and registered; copy that pattern exactly):

```sql
-- +goose Up
-- Migration: 00035_add_patchset_fingerprints
-- Description: Since-last-review triage. A patchset records the merge base it
-- was diffed against and whether per-file patch IDs were captured;
-- patchset_files holds them; pr_diffs keeps the patch ID used as the viewed
-- hash; last_reviewed_sha records which event set the baseline and when.
-- reviewed_at is TEXT in '%Y-%m-%dT%H:%M:%SZ' (UTC, second precision) so string
-- comparison is chronological.

ALTER TABLE patchsets ADD COLUMN merge_base_sha TEXT NOT NULL DEFAULT '';
ALTER TABLE patchsets ADD COLUMN has_fingerprints INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS patchset_files (
    pr_id           TEXT    NOT NULL,
    patchset_number INTEGER NOT NULL,
    path            TEXT    NOT NULL,
    patch_id        TEXT    NOT NULL,
    PRIMARY KEY (pr_id, patchset_number, path),
    FOREIGN KEY (pr_id, patchset_number) REFERENCES patchsets(pr_id, number) ON DELETE CASCADE
);

ALTER TABLE pr_diffs ADD COLUMN patch_id TEXT NOT NULL DEFAULT '';

ALTER TABLE last_reviewed_sha ADD COLUMN reviewed_at TEXT;
ALTER TABLE last_reviewed_sha ADD COLUMN source TEXT NOT NULL DEFAULT 'bistro_action';
UPDATE last_reviewed_sha SET reviewed_at = updated_at WHERE reviewed_at IS NULL;

-- +goose Down
-- This migration is irreversible.
```

- [ ] **Step 2: Update queries**

`queries/patchsets.sql` — replace `ListPatchsets`, add two queries (keep `ListCommentPatchsetNumbers`):

```sql
-- name: ListPatchsets :many
SELECT pr_id, number, head_sha, base_sha, pushed_at, merge_base_sha, has_fingerprints
FROM patchsets
WHERE pr_id = ?
ORDER BY number ASC;

-- name: GetCurrentPatchset :one
SELECT ps.pr_id, ps.number, ps.head_sha, ps.base_sha, ps.pushed_at, ps.merge_base_sha, ps.has_fingerprints
FROM patchsets ps
JOIN prs p ON p.id = ps.pr_id AND p.head_sha = ps.head_sha
WHERE ps.pr_id = ?
ORDER BY ps.number DESC
LIMIT 1;

-- name: ListPatchsetFiles :many
SELECT path, patch_id
FROM patchset_files
WHERE pr_id = ? AND patchset_number = ?
ORDER BY path;
```

`queries/last_reviewed.sql` — delete all four existing queries and replace with (the upsert is raw SQL in Go, Step 4, because it needs a conditional `DO UPDATE ... WHERE`):

```sql
-- name: GetLastReviewed :one
SELECT sha, patchset_number, reviewed_at, source
FROM last_reviewed_sha
WHERE pr_id = sqlc.arg(pr_id)
  AND account_provider = sqlc.arg(account_provider)
  AND account_login = sqlc.arg(account_login)
  AND login = sqlc.arg(login);
```

`queries/diff_store.sql` — add `patch_id` as the last column of `InsertDiff` (13 placeholders) and of the `ListDiffsForPR` select list.

Run: `cd go && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`
Expected: `internal/store/db/` regenerates; `models.go` `Patchset` gains `MergeBaseSha string`, `HasFingerprints int64`; `PrDiff` gains `PatchID`; `LastReviewedSha` gains `ReviewedAt sql.NullString`, `Source string`.

- [ ] **Step 3: Write the failing baseline test**

Replace the contents of `go/internal/store/last_reviewed_test.go` (its tests cover removed functions). `newTestDB` and `mustInsertPR` already exist in this package's tests.

```go
package store

import (
	"context"
	"testing"
	"time"
)

func TestRecordLastReviewedKeepsNewestEventAcrossSources(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lr-1")
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	record := func(source ReviewSource, head string, offset time.Duration) {
		t.Helper()
		event := ReviewEvent{PRID: "pr-lr-1", Login: "alice", HeadSHA: head,
			ReviewedAt: start.Add(offset), Source: source}
		if err := db.RecordLastReviewed(ctx, event); err != nil {
			t.Fatalf("record %s: %v", source, err)
		}
	}

	record(ReviewSourceGitHubReview, "h1", 0)
	record(ReviewSourceBistroAction, "h3", 3*time.Hour)
	record(ReviewSourceAllViewed, "h2", 2*time.Hour) // older than the stored event

	got, ok, err := db.GetLastReviewed(ctx, "pr-lr-1", "alice")
	if err != nil || !ok {
		t.Fatalf("GetLastReviewed: ok=%v err=%v", ok, err)
	}
	if got.Source != ReviewSourceBistroAction || got.SHA != "h3" || !got.ReviewedAt.Equal(start.Add(3*time.Hour)) {
		t.Fatalf("after older all_viewed: got %+v, want bistro_action h3", got)
	}

	record(ReviewSourceGitHubReview, "h4", 4*time.Hour)
	got, _, _ = db.GetLastReviewed(ctx, "pr-lr-1", "alice")
	if got.Source != ReviewSourceGitHubReview || got.SHA != "h4" {
		t.Fatalf("after newer github_review: got %+v, want github_review h4", got)
	}
}

func TestGetLastReviewedMissing(t *testing.T) {
	db := newTestDB(t)
	if _, ok, err := db.GetLastReviewed(context.Background(), "pr-none", "alice"); err != nil || ok {
		t.Fatalf("missing record: ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}
```

Run: `cd go && go test ./internal/store/ -run LastReviewed -v`
Expected: FAIL to compile (`undefined: ReviewSource`).

- [ ] **Step 4: Implement the store API**

Rewrite `go/internal/store/last_reviewed.go` (keep `localReviewAccountProvider` and `nullablePositiveInt`):

```go
// reviewedAtLayout is UTC at second precision so stored strings sort
// chronologically; it matches the strftime format migration 035 backfills.
const reviewedAtLayout = "2006-01-02T15:04:05Z"

// ReviewSource names the event that set a since-last-review baseline.
type ReviewSource string

const (
	ReviewSourceGitHubReview ReviewSource = "github_review"
	ReviewSourceBistroAction ReviewSource = "bistro_action"
	ReviewSourceAllViewed    ReviewSource = "all_viewed"
)

// ReviewEvent is one candidate since-last-review baseline.
// PatchsetNum 0 means unknown; readers then match the patchset by HeadSHA.
type ReviewEvent struct {
	PRID        string
	Login       string
	HeadSHA     string
	PatchsetNum int
	ReviewedAt  time.Time
	Source      ReviewSource
}

// LastReviewedRecord is the stored baseline for one PR and login.
type LastReviewedRecord struct {
	SHA         string
	PatchsetNum int
	ReviewedAt  time.Time
	Source      ReviewSource
}

// RecordLastReviewed stores event as the baseline unless a newer one is
// already stored. q is a *DB or the sync writer's *sql.Tx.
func RecordLastReviewed(ctx context.Context, q storedb.DBTX, event ReviewEvent) error {
	const query = `
		INSERT INTO last_reviewed_sha (
			pr_id, account_provider, account_login, login, sha,
			patchset_number, reviewed_at, source, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
		ON CONFLICT(pr_id, account_provider, account_login, login) DO UPDATE SET
			sha             = excluded.sha,
			patchset_number = excluded.patchset_number,
			reviewed_at     = excluded.reviewed_at,
			source          = excluded.source,
			updated_at      = excluded.updated_at
		WHERE last_reviewed_sha.reviewed_at IS NULL
		   OR excluded.reviewed_at > last_reviewed_sha.reviewed_at`
	if _, err := q.ExecContext(ctx, query,
		event.PRID, localReviewAccountProvider, event.Login, event.Login, event.HeadSHA,
		nullablePositiveInt(event.PatchsetNum), event.ReviewedAt.UTC().Format(reviewedAtLayout),
		string(event.Source),
	); err != nil {
		return fmt.Errorf("record last reviewed (%s, %s, %s): %w", event.PRID, event.Login, event.Source, err)
	}
	return nil
}

// RecordLastReviewed stores event as the baseline unless a newer one exists.
func (db *DB) RecordLastReviewed(ctx context.Context, event ReviewEvent) error {
	return RecordLastReviewed(ctx, db, event)
}

// GetLastReviewed returns the stored baseline for (prID, login).
func (db *DB) GetLastReviewed(ctx context.Context, prID, login string) (LastReviewedRecord, bool, error) {
	row, err := storedb.New(db).GetLastReviewed(ctx, storedb.GetLastReviewedParams{
		PrID: prID, AccountProvider: localReviewAccountProvider, AccountLogin: login, Login: login,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return LastReviewedRecord{}, false, nil
	}
	if err != nil {
		return LastReviewedRecord{}, false, fmt.Errorf("get last reviewed (%s, %s): %w", prID, login, err)
	}
	reviewedAt, err := time.Parse(reviewedAtLayout, row.ReviewedAt.String)
	if err != nil {
		return LastReviewedRecord{}, false, fmt.Errorf("parse last reviewed time (%s, %s): %w", prID, login, err)
	}
	return LastReviewedRecord{
		SHA: row.Sha, PatchsetNum: int(row.PatchsetNumber.Int64),
		ReviewedAt: reviewedAt, Source: ReviewSource(row.Source),
	}, true, nil
}
```

Adjust field names/types to what sqlc generated (e.g. `PatchsetNumber sql.NullInt64`). If `storedb.DBTX` is not satisfied by `*DB`, check `storedb.New(db)` usage — it already is.

In `go/internal/store/patchsets.go`: extend `Patchset` (struct above), map the two new columns in `patchsetFromRow` (`HasFingerprints: row.HasFingerprints != 0`), and add:

```go
// ListPatchsetFiles returns path → patch_id for one patchset.
func (db *DB) ListPatchsetFiles(ctx context.Context, prID string, number int) (map[string]string, error) {
	rows, err := storedb.New(db).ListPatchsetFiles(ctx, storedb.ListPatchsetFilesParams{
		PrID: prID, PatchsetNumber: int64(number),
	})
	if err != nil {
		return nil, fmt.Errorf("list patchset files (%s P%d): %w", prID, number, err)
	}
	files := make(map[string]string, len(rows))
	for _, row := range rows {
		files[row.Path] = row.PatchID
	}
	return files, nil
}

// CurrentPatchset returns the latest patchset at the PR's current head. The
// base tip does not identify a patchset: it moves without the PR changing.
func (db *DB) CurrentPatchset(ctx context.Context, prID string) (Patchset, bool, error) {
	row, err := storedb.New(db).GetCurrentPatchset(ctx, prID)
	if errors.Is(err, sql.ErrNoRows) {
		return Patchset{}, false, nil
	}
	if err != nil {
		return Patchset{}, false, fmt.Errorf("get current patchset (%s): %w", prID, err)
	}
	return patchsetFromRow(storedb.Patchset(row)), true, nil
}
```

(If sqlc returns `storedb.Patchset` directly for `GetCurrentPatchset`, drop the conversion.)

In `go/internal/store/diff_store.go`: add `PatchID string` to `store.DiffFile`, pass `PatchID: d.PatchID` in `SaveDiffs`, and map `row.PatchID` in the read path (`GetDiffs`).

- [ ] **Step 5: Run the store tests**

Run: `cd go && go test ./internal/store/ -v -run 'LastReviewed|Patchset|Diff'`
Expected: PASS.

- [ ] **Step 6: Move the executor to `RecordLastReviewed`**

In `go/internal/actions/executor.go`:

```go
// ReviewSHAWriter records the since-last-review baseline.
type ReviewSHAWriter interface {
	RecordLastReviewed(ctx context.Context, event store.ReviewEvent) error
}
```

In `recordLastReviewedSHA`, select only `head_sha` from `prs`, find the patchset by head (`SELECT number FROM patchsets WHERE pr_id = ? AND head_sha = ? ORDER BY number DESC LIMIT 1`, still best-effort on `sql.ErrNoRows`), then:

```go
	event := store.ReviewEvent{
		PRID: prID, Login: login, HeadSHA: headSHA, PatchsetNum: patchsetNum,
		ReviewedAt: time.Now(), Source: store.ReviewSourceBistroAction,
	}
	if err := ae.reviewSHAWriter.RecordLastReviewed(ctx, event); err != nil {
		ae.log.Warn("recordLastReviewedSHA: RecordLastReviewed failed",
			"pr_id", prID, "login", login, "patchset_num", patchsetNum, "error", err)
	}
```

Update `executor_test.go` mocks/assertions from `SetLastReviewedPatchset(..., baseSHA)` to `RecordLastReviewed(ctx, event)` (assert `event.Source == store.ReviewSourceBistroAction` and `event.HeadSHA`).

- [ ] **Step 7: Delete the Go range-diff path**

```bash
git rm go/internal/git/rangediff.go go/internal/git/rangediff_test.go go/internal/server/handler/rangediff.go
```

- Move `patchsetByNumber` (the only helper Task 4 needs) from the deleted `rangediff.go` into `handler/patchsets.go` verbatim.
- `handler/patchsets_test.go`: delete `mockRangeDiffRunner` and every `RangeDiff` test; keep the GET_PATCHSETS / MARK_FILE_VIEWED tests.
- `server/app.go`: remove `rangeDiffer` from `runStores`, its construction, the `range-diff probe` log line, and the `REQUEST_RANGE_DIFF` registration in `registerDiffAndCheckoutHandlers`.

Run: `cd go && grep -rn "RangeDiff\|rangeDiff\|range-diff" --include=*.go internal cmd`
Expected: no matches.

- [ ] **Step 8: Full Go verification**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 9: Commit** (app.go is pre-existing oversized; see README)

```bash
git add -A go/internal/store go/internal/actions go/internal/server go/internal/git
git commit -m "feat: store patchset fingerprints and timestamped review baselines

Adds migration 00035 and removes the git range-diff path, which never ran.
REQUEST_RANGE_DIFF has no handler until the UI moves to REQUEST_TRIAGE."
```
