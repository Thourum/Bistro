package store

import (
	"context"
	"testing"
	"time"
)

func TestRecordLastReviewedKeepsNewestEventAcrossSources(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lr-newest")
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	record := func(source ReviewSource, head string, offset time.Duration) {
		t.Helper()
		event := ReviewEvent{PRID: "pr-lr-newest", Login: "alice", HeadSHA: head,
			ReviewedAt: start.Add(offset), Source: source}
		if err := db.RecordLastReviewed(ctx, event); err != nil {
			t.Fatalf("record %s: %v", source, err)
		}
	}

	record(ReviewSourceGitHubReview, "h1", 0)
	record(ReviewSourceBistroAction, "h3", 3*time.Hour)
	record(ReviewSourceAllViewed, "h2", 2*time.Hour)

	got, ok, err := db.GetLastReviewed(ctx, "pr-lr-newest", "alice")
	if err != nil || !ok {
		t.Fatalf("GetLastReviewed: ok=%v err=%v", ok, err)
	}
	if got.Source != ReviewSourceBistroAction || got.SHA != "h3" || !got.ReviewedAt.Equal(start.Add(3*time.Hour)) {
		t.Fatalf("after older all_viewed: got %+v, want bistro_action h3", got)
	}

	record(ReviewSourceGitHubReview, "h4", 4*time.Hour)
	got, ok, err = db.GetLastReviewed(ctx, "pr-lr-newest", "alice")
	if err != nil || !ok {
		t.Fatalf("GetLastReviewed after newer event: ok=%v err=%v", ok, err)
	}
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

func TestRecordLastReviewedRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	record, ok, err := db.GetLastReviewed(ctx, "pr-lr-1", "alice")
	if err != nil || ok || record.SHA != "" {
		t.Fatalf("empty store: record=%+v ok=%v err=%v", record, ok, err)
	}

	mustInsertPR(t, db, "pr-lr-1")
	event := ReviewEvent{PRID: "pr-lr-1", Login: "alice", HeadSHA: "sha-abc",
		ReviewedAt: time.Now(), Source: ReviewSourceBistroAction}
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("record: %v", err)
	}
	record, ok, err = db.GetLastReviewed(ctx, event.PRID, event.Login)
	if err != nil || !ok || record.SHA != event.HeadSHA {
		t.Fatalf("after record: record=%+v ok=%v err=%v", record, ok, err)
	}

	event.HeadSHA = "sha-xyz"
	event.ReviewedAt = event.ReviewedAt.Add(time.Hour)
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	record, ok, err = db.GetLastReviewed(ctx, event.PRID, event.Login)
	if err != nil || !ok || record.SHA != event.HeadSHA {
		t.Fatalf("after overwrite: record=%+v ok=%v err=%v", record, ok, err)
	}
}

func TestGetLastReviewedDifferentLogins(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lr-2")

	for _, login := range []string{"alice", "bob"} {
		event := ReviewEvent{PRID: "pr-lr-2", Login: login, HeadSHA: "sha-" + login,
			ReviewedAt: time.Now(), Source: ReviewSourceBistroAction}
		if err := db.RecordLastReviewed(ctx, event); err != nil {
			t.Fatalf("record %s: %v", login, err)
		}
		record, found, err := db.GetLastReviewed(ctx, event.PRID, login)
		if err != nil || !found || record.SHA != event.HeadSHA {
			t.Fatalf("login %s: record=%+v found=%v err=%v", login, record, found, err)
		}
	}
	alice, found, err := db.GetLastReviewed(ctx, "pr-lr-2", "alice")
	if err != nil || !found || alice.SHA != "sha-alice" {
		t.Fatalf("alice after bob: record=%+v found=%v err=%v", alice, found, err)
	}
}

func TestGetLastReviewedPRIsolation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lr-3")
	mustInsertPR(t, db, "pr-lr-4")

	event := ReviewEvent{PRID: "pr-lr-3", Login: "alice", HeadSHA: "sha-3",
		ReviewedAt: time.Now(), Source: ReviewSourceBistroAction}
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("record: %v", err)
	}

	_, ok, err := db.GetLastReviewed(ctx, "pr-lr-4", "alice")
	if err != nil {
		t.Fatalf("get other PR: %v", err)
	}
	if ok {
		t.Fatal("pr isolation: pr-lr-4 should have no record")
	}
}

// ---- Patchset identity tests ----

func TestRecordLastReviewedPatchsetRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-p1")

	_, found, err := db.GetLastReviewed(ctx, "pr-p1", "alice")
	if err != nil || found {
		t.Fatalf("empty: found=%v err=%v", found, err)
	}

	event := ReviewEvent{PRID: "pr-p1", Login: "alice", HeadSHA: "sha-head", PatchsetNum: 2,
		ReviewedAt: time.Now(), Source: ReviewSourceBistroAction}
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("set: %v", err)
	}
	rec, found, err := db.GetLastReviewed(ctx, "pr-p1", "alice")
	if err != nil || !found {
		t.Fatalf("after set: found=%v err=%v", found, err)
	}
	if rec.PatchsetNum != 2 {
		t.Errorf("PatchsetNum: got %d want 2", rec.PatchsetNum)
	}
	if rec.SHA != "sha-head" {
		t.Errorf("SHA: got %q want sha-head", rec.SHA)
	}
	if rec.Source != event.Source || !rec.ReviewedAt.Equal(event.ReviewedAt.UTC().Truncate(time.Second)) {
		t.Errorf("event metadata: got %+v want %+v", rec, event)
	}
}

func TestRecordLastReviewedUnknownPatchset(t *testing.T) {
	// A review without a known patchset number must remain resolvable by head.
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-leg")

	event := ReviewEvent{PRID: "pr-leg", Login: "alice", HeadSHA: "sha-legacy",
		ReviewedAt: time.Now(), Source: ReviewSourceBistroAction}
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("set legacy: %v", err)
	}
	rec, found, err := db.GetLastReviewed(ctx, "pr-leg", "alice")
	if err != nil || !found {
		t.Fatalf("get patchset for legacy row: found=%v err=%v", found, err)
	}
	if rec.PatchsetNum != 0 {
		t.Errorf("legacy row: PatchsetNum should be 0 (unknown), got %d", rec.PatchsetNum)
	}
	if rec.SHA != "sha-legacy" {
		t.Errorf("legacy SHA: got %q want sha-legacy", rec.SHA)
	}
}

func TestRecordLastReviewedPatchsetReplacement(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-ups")

	event := ReviewEvent{PRID: "pr-ups", Login: "alice", HeadSHA: "sha-1", PatchsetNum: 1,
		ReviewedAt: time.Now(), Source: ReviewSourceBistroAction}
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("first record: %v", err)
	}
	event.HeadSHA = "sha-3"
	event.PatchsetNum = 3
	event.ReviewedAt = event.ReviewedAt.Add(time.Hour)
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("second record: %v", err)
	}
	rec, found, err := db.GetLastReviewed(ctx, "pr-ups", "alice")
	if err != nil || !found {
		t.Fatalf("get replacement: found=%v err=%v", found, err)
	}
	if rec.PatchsetNum != 3 || rec.SHA != "sha-3" {
		t.Errorf("after upsert: got num=%d sha=%q", rec.PatchsetNum, rec.SHA)
	}
}

func TestRecordLastReviewedStoresExplicitAccountNamespace(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-account-lr")

	event := ReviewEvent{PRID: "pr-account-lr", Login: "alice", HeadSHA: "sha-alice",
		ReviewedAt: time.Now(), Source: ReviewSourceBistroAction}
	if err := db.RecordLastReviewed(ctx, event); err != nil {
		t.Fatalf("set alice: %v", err)
	}

	var provider, accountLogin string
	if err := db.QueryRowContext(ctx,
		`SELECT account_provider, account_login FROM last_reviewed_sha WHERE pr_id = ? AND login = ?`,
		"pr-account-lr", "alice",
	).Scan(&provider, &accountLogin); err != nil {
		t.Fatalf("query account namespace: %v", err)
	}
	if provider != "github" || accountLogin != "alice" {
		t.Fatalf("account namespace = %s/%s, want github/alice", provider, accountLogin)
	}
}
