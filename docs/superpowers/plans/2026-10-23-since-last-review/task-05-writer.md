# Task 5: Writer — patchset identity, fingerprints, GitHub-review baseline

**Files:**
- Create: `go/internal/sync/writer_patchset.go` (new home of the patchset recorder)
- Create: `go/internal/sync/writer_patchset_test.go`
- Modify: `go/internal/sync/writer_details.go` (delete `recordPatchsetIfChanged`; add `patch_id` to `writeDiffs` insert)
- Modify: `go/internal/sync/writer.go` (`writeCriticalPRData`, delete `readCurrentPatchsetNumber`, `writeAfterSavepoint`)
- Modify: `go/internal/sync/fetcher.go` (`PRFetchResult.Reviews`, `fetchReviews`, `storeDiffFiles`)
- Modify: `go/internal/provider/models.go` (`Review.CommitID`), `go/internal/provider/github/reviews.go`
- Modify: `go/internal/sync/orchestrator_anchors.go` (`selectDiffBaseSHA`)
- Modify: `go/internal/server/handler/comments.go` (current-patchset join)

**Interfaces:**
- Consumes: `PRFetchResult.MergeBaseSHA` (Task 2), `git.DiffFile.PatchID` (Task 1), `store.DiffFile.PatchID`, `store.RecordLastReviewed`, `store.ReviewEvent`, `store.ReviewSourceGitHubReview`, migration 00035 columns (Task 3).
- Produces:
  - `provider.Review.CommitID string` (GitHub `commit_id`).
  - `PRFetchResult.Reviews []*provider.Review`.
  - Rule used everywhere: **current patchset = highest-numbered patchset whose `head_sha` equals the PR's head.** A new patchset is recorded when the head changes, or when both the stored and the new merge base are known and differ. A base-tip move alone records nothing.
  - Fingerprints (`patchset_files` rows + `has_fingerprints = 1`) are written the first time a diff is computed for a patchset, including backfilling patchsets recorded earlier without them.

- [ ] **Step 1: Write the failing tests**

`go/internal/sync/writer_patchset_test.go` (`newSyncTestDB`, `mentionedTestCurrentUserReader` exist in `writer_test.go`):

```go
package sync

import (
	"context"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

func patchsetTestPR(baseSHA, headSHA string) *provider.PR {
	return &provider.PR{
		ID: "github:o/r:9", Provider: "github", Owner: "o", Repo: "r", Number: 9,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: baseSHA, HeadSHA: headSHA,
	}
}

func diffResult(pr *provider.PR, mergeBase string, patchIDs map[string]string) *PRFetchResult {
	files := make([]store.DiffFile, 0, len(patchIDs))
	for path, patchID := range patchIDs {
		files = append(files, store.DiffFile{FilePath: path, Patch: "p", PatchID: patchID})
	}
	return &PRFetchResult{PR: pr, DiffComputed: true, DiffFiles: files, MergeBaseSHA: mergeBase}
}

func TestBaseTipMoveDoesNotRecordPatchset(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	ctx := context.Background()
	write := func(result *PRFetchResult) {
		t.Helper()
		if err := w.WriteSyncResults([]*PRFetchResult{result}); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	write(diffResult(patchsetTestPR("tip-1", "h1"), "mb-1", map[string]string{"a.go": "pa"}))
	write(diffResult(patchsetTestPR("tip-2", "h1"), "mb-1", map[string]string{"a.go": "pa"}))
	patchsets, err := db.ListPatchsets(ctx, "github:o/r:9")
	if err != nil || len(patchsets) != 1 {
		t.Fatalf("after base-tip move: %d patchsets (err %v), want 1", len(patchsets), err)
	}

	write(diffResult(patchsetTestPR("tip-2", "h2"), "mb-1", map[string]string{"a.go": "pa2", "b.go": "pb"}))
	patchsets, _ = db.ListPatchsets(ctx, "github:o/r:9")
	if len(patchsets) != 2 || patchsets[1].MergeBaseSHA != "mb-1" || !patchsets[1].HasFingerprints {
		t.Fatalf("after head change: %+v, want P2 with merge base and fingerprints", patchsets)
	}
	files, err := db.ListPatchsetFiles(ctx, "github:o/r:9", 2)
	if err != nil || files["a.go"] != "pa2" || files["b.go"] != "pb" {
		t.Fatalf("P2 files = %v (err %v)", files, err)
	}
}

func TestPatchsetWithoutDiffIsBackfilled(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	pr := patchsetTestPR("tip-1", "h1")
	if err := w.WriteSyncResults([]*PRFetchResult{{PR: pr}}); err != nil {
		t.Fatalf("write without diff: %v", err)
	}
	if err := w.WriteSyncResults([]*PRFetchResult{diffResult(pr, "mb-1", map[string]string{"a.go": "pa"})}); err != nil {
		t.Fatalf("write with diff: %v", err)
	}
	patchsets, _ := db.ListPatchsets(context.Background(), pr.ID)
	if len(patchsets) != 1 || patchsets[0].MergeBaseSHA != "mb-1" || !patchsets[0].HasFingerprints {
		t.Fatalf("patchsets = %+v, want one backfilled P1", patchsets)
	}
}

func TestWriterRecordsLatestGitHubReviewAsBaseline(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "me", ok: true})
	reviewedAt := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	result := &PRFetchResult{PR: patchsetTestPR("tip-1", "h2"), Reviews: []*provider.Review{
		{Author: "me", State: "approved", CommitID: "h1", SubmittedAt: reviewedAt.Add(-time.Hour)},
		{Author: "Me", State: "commented", CommitID: "h2", SubmittedAt: reviewedAt},
		{Author: "me", State: "pending", CommitID: "h3"},
		{Author: "other", State: "approved", CommitID: "h3", SubmittedAt: reviewedAt.Add(time.Hour)},
	}}
	if err := w.WriteSyncResults([]*PRFetchResult{result}); err != nil {
		t.Fatalf("write: %v", err)
	}
	record, ok, err := db.GetLastReviewed(context.Background(), result.PR.ID, "me")
	if err != nil || !ok {
		t.Fatalf("GetLastReviewed: ok=%v err=%v", ok, err)
	}
	if record.SHA != "h2" || record.Source != store.ReviewSourceGitHubReview || !record.ReviewedAt.Equal(reviewedAt) {
		t.Fatalf("baseline = %+v, want github_review at h2", record)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd go && go test ./internal/sync/ -run 'BaseTipMove|Backfilled|GitHubReviewAsBaseline' -v`
Expected: FAIL to compile (`Reviews` / `CommitID` unknown fields).

- [ ] **Step 3: Carry reviews and patch IDs through the fetch**

- `provider/models.go` `Review`: add `CommitID string // head commit the review was submitted against`.
- `provider/github/reviews.go` `listReviews`: set `CommitID: ghReview.GetCommitID(),`.
- `sync/fetcher.go`: add to `PRFetchResult` `Reviews []*provider.Review // submitted reviews; the writer derives the user's GitHub-review baseline` and set `result.Reviews = reviews` in `fetchReviews`. In `storeDiffFiles` add `PatchID: file.PatchID,`.
- `sync/writer_details.go` `writeDiffs`: add `patch_id` as the 13th column/placeholder and pass `diff.PatchID`.

- [ ] **Step 4: Implement the patchset recorder**

Delete `recordPatchsetIfChanged` from `writer_details.go` and `readCurrentPatchsetNumber` from `writer.go`. Create `go/internal/sync/writer_patchset.go`:

```go
package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store"
)

type patchsetWrite struct {
	pr        *provider.PR
	mergeBase string           // "" when unknown
	files     []store.DiffFile // nil when the diff was not computed this sync
}

type latestPatchset struct {
	number    int
	headSHA   string
	mergeBase string
}

// recordPatchset returns the PR's current patchset number, recording a new
// patchset when the head or a known merge base changed. A base tip that moves
// without changing the merge base is the same patchset.
func (w *Writer) recordPatchset(ctx context.Context, tx *sql.Tx, write patchsetWrite) (int, error) {
	latest, found, err := readLatestPatchset(ctx, tx, write.pr.ID)
	if err != nil {
		return 0, err
	}
	number := latest.number
	if found && !isNewPatchset(latest, write) {
		err = backfillMergeBase(ctx, tx, write, number)
	} else {
		number, err = insertPatchset(ctx, tx, write, latest.number+1)
	}
	if err != nil {
		return 0, err
	}
	if write.files == nil {
		return number, nil
	}
	return number, writeFingerprintsOnce(ctx, tx, write, number)
}

func isNewPatchset(latest latestPatchset, write patchsetWrite) bool {
	if latest.headSHA != write.pr.HeadSHA {
		return true
	}
	return write.mergeBase != "" && latest.mergeBase != "" && latest.mergeBase != write.mergeBase
}

func readLatestPatchset(ctx context.Context, tx *sql.Tx, prID string) (latestPatchset, bool, error) {
	var latest latestPatchset
	err := tx.QueryRowContext(ctx,
		`SELECT number, head_sha, merge_base_sha FROM patchsets WHERE pr_id = ? ORDER BY number DESC LIMIT 1`,
		prID,
	).Scan(&latest.number, &latest.headSHA, &latest.mergeBase)
	if errors.Is(err, sql.ErrNoRows) {
		return latestPatchset{}, false, nil
	}
	if err != nil {
		return latestPatchset{}, false, fmt.Errorf("query latest patchset for PR %s: %w", prID, err)
	}
	return latest, true, nil
}

// insertPatchset inserts P<number>. UNIQUE(pr_id, base_sha, head_sha) ignores
// a return to an exact earlier (base tip, head) pair; that row stays current.
func insertPatchset(ctx context.Context, tx *sql.Tx, write patchsetWrite, number int) (int, error) {
	pr := write.pr
	inserted, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO patchsets (pr_id, number, head_sha, base_sha, merge_base_sha, pushed_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		pr.ID, number, pr.HeadSHA, pr.BaseSHA, write.mergeBase, time.Now())
	if err != nil {
		return 0, fmt.Errorf("insert patchset %s P%d: %w", pr.ID, number, err)
	}
	if rows, err := inserted.RowsAffected(); err != nil || rows == 1 {
		return number, err
	}
	var existing int
	if err := tx.QueryRowContext(ctx,
		`SELECT number FROM patchsets WHERE pr_id = ? AND base_sha = ? AND head_sha = ?`,
		pr.ID, pr.BaseSHA, pr.HeadSHA,
	).Scan(&existing); err != nil {
		return 0, fmt.Errorf("read existing patchset %s at %s: %w", pr.ID, truncateSHA(pr.HeadSHA), err)
	}
	return existing, nil
}

func backfillMergeBase(ctx context.Context, tx *sql.Tx, write patchsetWrite, number int) error {
	if write.mergeBase == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE patchsets SET merge_base_sha = ? WHERE pr_id = ? AND number = ? AND merge_base_sha = ''`,
		write.mergeBase, write.pr.ID, number,
	); err != nil {
		return fmt.Errorf("backfill merge base %s P%d: %w", write.pr.ID, number, err)
	}
	return nil
}

// writeFingerprintsOnce stores per-file patch IDs the first time a diff is
// computed for a patchset; the same head and merge base always yield the same
// diff, so later syncs have nothing to add.
func writeFingerprintsOnce(ctx context.Context, tx *sql.Tx, write patchsetWrite, number int) error {
	marked, err := tx.ExecContext(ctx,
		`UPDATE patchsets SET has_fingerprints = 1 WHERE pr_id = ? AND number = ? AND has_fingerprints = 0`,
		write.pr.ID, number)
	if err != nil {
		return fmt.Errorf("mark fingerprints %s P%d: %w", write.pr.ID, number, err)
	}
	if rows, err := marked.RowsAffected(); err != nil || rows == 0 {
		return err
	}
	for _, file := range write.files {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO patchset_files (pr_id, patchset_number, path, patch_id) VALUES (?, ?, ?, ?)`,
			write.pr.ID, number, file.FilePath, file.PatchID,
		); err != nil {
			return fmt.Errorf("insert fingerprint %s P%d %s: %w", write.pr.ID, number, file.FilePath, err)
		}
	}
	return nil
}

// recordGitHubReviewBaseline offers the current user's latest submitted
// review (browser, gh, or Bistro) as the since-last-review baseline.
// Best-effort: the PR write never fails because of it.
func (w *Writer) recordGitHubReviewBaseline(ctx context.Context, batch *writerBatch, result *PRFetchResult) {
	if w.currentUser == nil || len(result.Reviews) == 0 {
		return
	}
	login, ok, err := w.currentUser.GetCurrentUser(ctx, "github")
	if err != nil || !ok || login == "" {
		return // No user means no baseline to attribute; mentions handle the logging.
	}
	review := latestSubmittedReview(result.Reviews, login)
	if review == nil {
		return
	}
	if err := store.RecordLastReviewed(ctx, batch.tx, store.ReviewEvent{
		PRID: result.PR.ID, Login: login, HeadSHA: review.CommitID,
		ReviewedAt: review.SubmittedAt, Source: store.ReviewSourceGitHubReview,
	}); err != nil {
		batch.log.Warn("record GitHub review baseline failed", "pr_id", result.PR.ID, "error", err)
	}
}

func latestSubmittedReview(reviews []*provider.Review, login string) *provider.Review {
	var latest *provider.Review
	for _, review := range reviews {
		isSubmitted := review != nil && review.State != "pending" && review.CommitID != "" && !review.SubmittedAt.IsZero()
		if !isSubmitted || !strings.EqualFold(review.Author, login) {
			continue
		}
		if latest == nil || review.SubmittedAt.After(latest.SubmittedAt) {
			latest = review
		}
	}
	return latest
}
```

Check: `truncateSHA` exists in `fetcher.go`; `batch.log` and `batch.tx` exist on `writerBatch`. If `GetCurrentUser` errors here, the mentions path already logs it — do not log twice.

- [ ] **Step 5: Wire it into the writer**

In `writer.go` `writeCriticalPRData`, replace the `recordPatchsetIfChanged` call and the trailing `readCurrentPatchsetNumber` with (keep `writeDiffs` where it is):

```go
	var files []store.DiffFile
	if result.DiffComputed {
		files = result.DiffFiles
		if files == nil {
			files = []store.DiffFile{} // Computed-but-empty still records "no files".
		}
	}
	number, err := w.recordPatchset(ctx, tx, patchsetWrite{pr: pr, mergeBase: result.MergeBaseSHA, files: files})
	if err != nil {
		return counts, err
	}
	counts.patchsetNumber = number
```

and end the function with `return counts, nil`. In `writeAfterSavepoint` add first: `w.recordGitHubReviewBaseline(ctx, batch, result)`.

- [ ] **Step 6: Head-only current patchset everywhere else**

- `orchestrator_anchors.go` `selectDiffBaseSHA`, current-position branch — match on head only and take the highest number:

```go
	if options.isCurrentPosition {
		current := 0
		for _, patchset := range options.patchsets {
			if patchset.HeadSHA == options.prHeadSHA {
				current = patchset.Number
			}
		}
		return options.prBaseSHA, current, false
	}
```

  and in the original-position branch return the patchset's diff base: `patchset.MergeBaseSHA` when non-empty, else `patchset.BaseSHA`. (`options.prBaseSHA` is already the diff base since Task 2.)
- `handler/comments.go`: replace the join condition `AND ps.head_sha = p.head_sha AND ps.base_sha = p.base_sha` with
  `AND ps.number = (SELECT MAX(x.number) FROM patchsets x WHERE x.pr_id = c.pr_id AND x.head_sha = p.head_sha)`.

Run: `cd go && grep -rn "base_sha = ?" --include=*.go internal | grep -i patchset`
Expected: only `insertPatchset`'s UNIQUE-conflict lookup.

- [ ] **Step 7: Run tests**

Run: `cd go && go test ./internal/sync/ -run 'BaseTipMove|Backfilled|GitHubReviewAsBaseline|RecordPatchset' -v`
Expected: PASS (the existing `TestRecordPatchsetIfChanged_*` tests keep passing: they go through `WriteSyncResults`).

- [ ] **Step 8: Full Go verification**

Run: `cd go && go vet ./... && go test -race ./...`
Expected: PASS. Anchor tests asserting a base+head patchset match where only the base differed must be updated to head-only semantics; explain in the commit body.

- [ ] **Step 9: Commit**

```bash
git add go/internal/sync go/internal/provider go/internal/server/handler/comments.go
git commit -m "feat: record patchsets by head and merge base with file fingerprints

A base-branch tip move no longer creates a patchset. The current user's
latest GitHub review is offered as the since-last-review baseline."
```
