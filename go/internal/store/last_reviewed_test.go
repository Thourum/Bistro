package store

import (
	"context"
	"testing"
)

func TestSetGetLastReviewedSHA_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	sha, ok, err := db.GetLastReviewedSHA(ctx, "pr-lr-1", "alice")
	if err != nil {
		t.Fatalf("get on empty: %v", err)
	}
	if ok || sha != "" {
		t.Fatalf("empty store: got (%q, %v)", sha, ok)
	}

	mustInsertPR(t, db, "pr-lr-1")
	if err := db.SetLastReviewedSHA(ctx, "pr-lr-1", "alice", "sha-abc"); err != nil {
		t.Fatalf("set: %v", err)
	}
	sha, ok, err = db.GetLastReviewedSHA(ctx, "pr-lr-1", "alice")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok || sha != "sha-abc" {
		t.Fatalf("after set: got (%q, %v)", sha, ok)
	}

	if err := db.SetLastReviewedSHA(ctx, "pr-lr-1", "alice", "sha-xyz"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	sha, ok, _ = db.GetLastReviewedSHA(ctx, "pr-lr-1", "alice")
	if !ok || sha != "sha-xyz" {
		t.Fatalf("after overwrite: got (%q, %v)", sha, ok)
	}
}

func TestGetLastReviewedSHA_DifferentLogins(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lr-2")

	if err := db.SetLastReviewedSHA(ctx, "pr-lr-2", "alice", "sha-alice"); err != nil {
		t.Fatalf("set alice: %v", err)
	}
	if err := db.SetLastReviewedSHA(ctx, "pr-lr-2", "bob", "sha-bob"); err != nil {
		t.Fatalf("set bob: %v", err)
	}

	shaA, _, _ := db.GetLastReviewedSHA(ctx, "pr-lr-2", "alice")
	shaB, _, _ := db.GetLastReviewedSHA(ctx, "pr-lr-2", "bob")
	if shaA != "sha-alice" || shaB != "sha-bob" {
		t.Fatalf("login isolation: alice=%q bob=%q", shaA, shaB)
	}
}

func TestGetLastReviewedSHA_PRIsolation(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-lr-3")
	mustInsertPR(t, db, "pr-lr-4")

	if err := db.SetLastReviewedSHA(ctx, "pr-lr-3", "alice", "sha-3"); err != nil {
		t.Fatalf("set: %v", err)
	}

	_, ok, _ := db.GetLastReviewedSHA(ctx, "pr-lr-4", "alice")
	if ok {
		t.Fatal("pr isolation: pr-lr-4 should have no record")
	}
}

// ---- Full patchset identity tests (Blocker 1 fix) ----

func TestSetGetLastReviewedPatchset_RoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-p1")

	_, found, err := db.GetLastReviewedPatchset(ctx, "pr-p1", "alice")
	if err != nil || found {
		t.Fatalf("empty: found=%v err=%v", found, err)
	}

	if err := db.SetLastReviewedPatchset(ctx, "pr-p1", "alice", 2, "sha-head", "sha-base"); err != nil {
		t.Fatalf("set: %v", err)
	}
	rec, found, err := db.GetLastReviewedPatchset(ctx, "pr-p1", "alice")
	if err != nil || !found {
		t.Fatalf("after set: found=%v err=%v", found, err)
	}
	if rec.PatchsetNum != 2 {
		t.Errorf("PatchsetNum: got %d want 2", rec.PatchsetNum)
	}
	if rec.SHA != "sha-head" {
		t.Errorf("SHA: got %q want sha-head", rec.SHA)
	}
	if rec.BaseSHA != "sha-base" {
		t.Errorf("BaseSHA: got %q want sha-base", rec.BaseSHA)
	}
}

func TestSetGetLastReviewedPatchset_LegacyPath(t *testing.T) {
	// SetLastReviewedSHA writes patchset_number=NULL (legacy).
	// GetLastReviewedPatchset should return PatchsetNum==0 for such rows.
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-leg")

	if err := db.SetLastReviewedSHA(ctx, "pr-leg", "alice", "sha-legacy"); err != nil {
		t.Fatalf("set legacy: %v", err)
	}
	rec, found, err := db.GetLastReviewedPatchset(ctx, "pr-leg", "alice")
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

func TestSetLastReviewedPatchset_Upsert(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	mustInsertPR(t, db, "pr-ups")

	if err := db.SetLastReviewedPatchset(ctx, "pr-ups", "alice", 1, "sha-1", "base-1"); err != nil {
		t.Fatalf("first set: %v", err)
	}
	if err := db.SetLastReviewedPatchset(ctx, "pr-ups", "alice", 3, "sha-3", "base-2"); err != nil {
		t.Fatalf("second set: %v", err)
	}
	rec, _, _ := db.GetLastReviewedPatchset(ctx, "pr-ups", "alice")
	if rec.PatchsetNum != 3 || rec.SHA != "sha-3" {
		t.Errorf("after upsert: got num=%d sha=%q", rec.PatchsetNum, rec.SHA)
	}
}
