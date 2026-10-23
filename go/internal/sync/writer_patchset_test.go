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
	patchsets, err = db.ListPatchsets(ctx, "github:o/r:9")
	if err != nil || len(patchsets) != 2 || patchsets[1].MergeBaseSHA != "mb-1" || !patchsets[1].HasFingerprints {
		t.Fatalf("after head change: %+v (err %v), want P2 with merge base and fingerprints", patchsets, err)
	}
	files, err := db.ListPatchsetFiles(ctx, "github:o/r:9", 2)
	if err != nil || files["a.go"] != "pa2" || files["b.go"] != "pb" {
		t.Fatalf("P2 files = %v (err %v)", files, err)
	}
	cached, err := store.NewDiffStore(db.DB).GetDiffs("github:o/r:9")
	if err != nil || len(cached) != 2 {
		t.Fatalf("cached diffs = %v (err %v)", cached, err)
	}
	for _, file := range cached {
		if file.PatchID != files[file.FilePath] {
			t.Errorf("cached patch ID for %s = %q, want %q", file.FilePath, file.PatchID, files[file.FilePath])
		}
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
	patchsets, err := db.ListPatchsets(context.Background(), pr.ID)
	if err != nil || len(patchsets) != 1 || patchsets[0].MergeBaseSHA != "mb-1" || !patchsets[0].HasFingerprints {
		t.Fatalf("patchsets = %+v (err %v), want one backfilled P1", patchsets, err)
	}
	files, err := db.ListPatchsetFiles(context.Background(), pr.ID, 1)
	if err != nil || files["a.go"] != "pa" {
		t.Fatalf("backfilled files = %v (err %v), want a.go fingerprint", files, err)
	}
}

func TestRecordPatchsetReturnToEarlierPairUsesLatestMatchingHead(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	ctx := context.Background()
	results := []*PRFetchResult{
		diffResult(patchsetTestPR("tip-1", "h1"), "mb-1", map[string]string{"a.go": "pa"}),
		diffResult(patchsetTestPR("tip-2", "h1"), "mb-2", map[string]string{"a.go": "pa2"}),
		diffResult(patchsetTestPR("tip-3", "h2"), "mb-2", map[string]string{"a.go": "pa3"}),
		diffResult(patchsetTestPR("tip-1", "h1"), "mb-1", map[string]string{"a.go": "pa"}),
	}
	position := 1
	results[3].Comments = []*provider.Comment{{
		ID: "return-comment", Author: "reviewer", Kind: "review", Position: &position,
	}}
	for _, result := range results {
		if err := w.WriteSyncResults([]*PRFetchResult{result}); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	patchsets, err := db.ListPatchsets(ctx, results[0].PR.ID)
	if err != nil || len(patchsets) != 3 {
		t.Fatalf("patchsets = %+v (err %v), want exact-pair return to reuse a patchset", patchsets, err)
	}
	current, ok, err := db.CurrentPatchset(ctx, results[0].PR.ID)
	if err != nil || !ok || current.Number != 2 {
		t.Fatalf("current = %+v, ok=%v err=%v; want latest matching-head P2", current, ok, err)
	}
	var commentPatchset int
	if err := db.QueryRowContext(ctx, `SELECT patchset_number FROM comments WHERE id = ?`, "return-comment").Scan(&commentPatchset); err != nil {
		t.Fatalf("read comment patchset: %v", err)
	}
	if commentPatchset != current.Number {
		t.Fatalf("writer returned P%d for current comment, want current P%d", commentPatchset, current.Number)
	}
}

func TestRecordPatchsetKnownMergeBaseChangeAndFingerprintOnce(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	ctx := context.Background()
	results := []*PRFetchResult{
		diffResult(patchsetTestPR("tip-1", "h1"), "mb-1", map[string]string{"a.go": "pa"}),
		diffResult(patchsetTestPR("tip-2", "h1"), "mb-2", map[string]string{"b.go": "pb"}),
		diffResult(patchsetTestPR("tip-2", "h1"), "mb-2", map[string]string{"b.go": "pb"}),
		{PR: patchsetTestPR("tip-3", "h1")},
	}
	for _, result := range results {
		if err := w.WriteSyncResults([]*PRFetchResult{result}); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	patchsets, err := db.ListPatchsets(ctx, results[0].PR.ID)
	if err != nil || len(patchsets) != 2 || patchsets[1].MergeBaseSHA != "mb-2" {
		t.Fatalf("patchsets = %+v (err %v), want P2 for known merge-base change", patchsets, err)
	}
	for number, expected := range map[int]map[string]string{1: {"a.go": "pa"}, 2: {"b.go": "pb"}} {
		files, err := db.ListPatchsetFiles(ctx, results[0].PR.ID, number)
		if err != nil || len(files) != len(expected) {
			t.Fatalf("P%d files = %v (err %v)", number, files, err)
		}
		for path, patchID := range expected {
			if files[path] != patchID {
				t.Errorf("P%d %s = %q, want %q", number, path, files[path], patchID)
			}
		}
	}
	current, ok, err := db.CurrentPatchset(ctx, results[0].PR.ID)
	if err != nil || !ok || current.Number != 2 {
		t.Fatalf("current = %+v, ok=%v err=%v; want P2 despite base-tip move", current, ok, err)
	}
}

func TestRecordPatchsetEmptyDiffHasFingerprints(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	result := &PRFetchResult{PR: patchsetTestPR("tip-1", "h1"), DiffComputed: true, MergeBaseSHA: "mb-1"}
	if err := w.WriteSyncResults([]*PRFetchResult{result}); err != nil {
		t.Fatalf("write: %v", err)
	}
	current, ok, err := db.CurrentPatchset(context.Background(), result.PR.ID)
	if err != nil || !ok || !current.HasFingerprints {
		t.Fatalf("current = %+v, ok=%v err=%v; want computed-empty fingerprints", current, ok, err)
	}
	files, err := db.ListPatchsetFiles(context.Background(), result.PR.ID, current.Number)
	if err != nil || len(files) != 0 {
		t.Fatalf("files = %v (err %v), want no files", files, err)
	}
}

func TestRecordPatchsetFingerprintFailureRollsBackPR(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	ctx := context.Background()
	initial := diffResult(patchsetTestPR("tip-1", "h1"), "mb-1", map[string]string{"a.go": "pa"})
	if err := w.WriteSyncResults([]*PRFetchResult{initial}); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_fingerprints BEFORE INSERT ON patchset_files
		BEGIN SELECT RAISE(ABORT, 'fingerprint insert failed'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	failed := diffResult(patchsetTestPR("tip-2", "h2"), "mb-2", map[string]string{"b.go": "pb"})
	if err := w.WriteSyncResults([]*PRFetchResult{failed}); err != nil {
		t.Fatalf("batch should skip failed PR: %v", err)
	}
	current, ok, err := db.CurrentPatchset(ctx, initial.PR.ID)
	if err != nil || !ok || current.Number != 1 || current.HeadSHA != "h1" {
		t.Fatalf("current = %+v, ok=%v err=%v; want original PR and P1", current, ok, err)
	}
	patchsets, err := db.ListPatchsets(ctx, initial.PR.ID)
	if err != nil || len(patchsets) != 1 {
		t.Fatalf("patchsets = %+v (err %v), failed P2 must roll back", patchsets, err)
	}
	cached, err := store.NewDiffStore(db.DB).GetDiffs(initial.PR.ID)
	if err != nil || len(cached) != 1 || cached[0].PatchID != "pa" {
		t.Fatalf("cached diffs = %+v (err %v), want original cache", cached, err)
	}
}

func TestWriterGitHubReviewBaselinePreservesNewerLocalReview(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "me", ok: true})
	ctx := context.Background()
	pr := patchsetTestPR("tip-1", "h1")
	mustWritePR(t, w, pr)
	reviewedAt := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	if err := db.RecordLastReviewed(ctx, store.ReviewEvent{
		PRID: pr.ID, Login: "me", HeadSHA: "local-head", ReviewedAt: reviewedAt,
		Source: store.ReviewSourceAllViewed,
	}); err != nil {
		t.Fatalf("record local baseline: %v", err)
	}
	result := &PRFetchResult{PR: pr, Reviews: []*provider.Review{{
		Author: "me", State: "approved", CommitID: "older-head", SubmittedAt: reviewedAt.Add(-time.Hour),
	}}}
	if err := w.WriteSyncResults([]*PRFetchResult{result}); err != nil {
		t.Fatalf("write: %v", err)
	}
	record, ok, err := db.GetLastReviewed(ctx, pr.ID, "me")
	if err != nil || !ok || record.SHA != "local-head" || record.Source != store.ReviewSourceAllViewed {
		t.Fatalf("baseline = %+v, ok=%v err=%v; want newer local baseline", record, ok, err)
	}
}

func TestWriterGitHubReviewBaselineFailureDoesNotDiscardPR(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "me", ok: true})
	if _, err := db.Exec(`CREATE TRIGGER reject_review_baseline BEFORE INSERT ON last_reviewed_sha
		BEGIN SELECT RAISE(ABORT, 'baseline insert failed'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	result := &PRFetchResult{PR: patchsetTestPR("tip-1", "h1"), Reviews: []*provider.Review{{
		Author: "me", State: "approved", CommitID: "h1",
		SubmittedAt: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC),
	}}}
	if err := w.WriteSyncResults([]*PRFetchResult{result}); err != nil {
		t.Fatalf("write: %v", err)
	}
	current, ok, err := db.CurrentPatchset(context.Background(), result.PR.ID)
	if err != nil || !ok || current.HeadSHA != "h1" {
		t.Fatalf("current = %+v, ok=%v err=%v; baseline failure must preserve PR", current, ok, err)
	}
}

func TestWriterRecordsLatestGitHubReviewAsBaseline(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db).WithCurrentUser(mentionedTestCurrentUserReader{login: "me", ok: true})
	reviewedAt := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	result := &PRFetchResult{PR: patchsetTestPR("tip-1", "h2"), Reviews: []*provider.Review{
		{Author: "me", State: "approved", CommitID: "h1", SubmittedAt: reviewedAt.Add(-time.Hour)},
		{Author: "Me", State: "commented", CommitID: "h2", SubmittedAt: reviewedAt},
		{Author: "me", State: "approved", CommitID: "older", SubmittedAt: reviewedAt.Add(-time.Hour)},
		nil,
		{Author: "me", State: "pending", CommitID: "h3", SubmittedAt: reviewedAt.Add(time.Hour)},
		{Author: "me", State: "approved", SubmittedAt: reviewedAt.Add(time.Hour)},
		{Author: "me", State: "commented", CommitID: "h3"},
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
