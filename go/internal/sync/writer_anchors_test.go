package sync

import (
	"context"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
)

// ── Anchor write-back tests ───────────────────────────────────────────────

func TestWriteAnchorUpdates_WritesAnchoredLine(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:50", 50, "sha-head", "sha-base")
	pos := 5
	// Write PR with a review comment.
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID:        "rc-1",
			PRID:      pr.ID,
			Author:    "alice",
			Body:      "nit",
			Kind:      "review",
			CommitSHA: "sha-commit",
			Position:  &pos,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}},
	}}); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	// Now write with AnchorUpdates: anchored at line 12.
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: makePR("github:o/r:50", 50, "sha-head", "sha-base"),
		AnchorUpdates: []git.AnchorResult{{
			CommentID:    "rc-1",
			AnchoredLine: 12,
			HeadSHA:      "sha-head",
			Anchored:     true,
		}},
	}}); err != nil {
		t.Fatalf("anchor write: %v", err)
	}

	var anchoredLine *int
	var anchorHeadSHA *string
	var outdated int
	db.QueryRow(`SELECT anchored_line, anchor_head_sha, outdated FROM comments WHERE id = 'rc-1'`).
		Scan(&anchoredLine, &anchorHeadSHA, &outdated) //nolint:errcheck
	if anchoredLine == nil || *anchoredLine != 12 {
		t.Errorf("anchored_line: got %v want 12", anchoredLine)
	}
	if anchorHeadSHA == nil || *anchorHeadSHA != "sha-head" {
		t.Errorf("anchor_head_sha: got %v want sha-head", anchorHeadSHA)
	}
	if outdated != 0 {
		t.Errorf("outdated: got %d want 0", outdated)
	}
}

func TestWriteAnchorUpdates_OutdatedComment(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:51", 51, "sha-h51", "sha-b51")
	pos := 3
	w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID: "rc-2", PRID: pr.ID, Author: "bob", Body: "q",
			Kind: "review", CommitSHA: "sha-c51", Position: &pos,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
	}}) //nolint:errcheck

	w.WriteSyncResults([]*PRFetchResult{{
		PR: makePR("github:o/r:51", 51, "sha-h51", "sha-b51"),
		AnchorUpdates: []git.AnchorResult{{
			CommentID:        "rc-2",
			HeadSHA:          "sha-h51",
			Outdated:         true,
			OutdatedNearLine: 7,
		}},
	}}) //nolint:errcheck

	var anchoredLine *int
	var outdated int
	var nearLineDB *int
	db.QueryRow(`SELECT anchored_line, outdated, outdated_near_line FROM comments WHERE id = 'rc-2'`).
		Scan(&anchoredLine, &outdated, &nearLineDB) //nolint:errcheck
	if anchoredLine != nil {
		t.Errorf("anchored_line should be NULL when outdated, got %d", *anchoredLine)
	}
	if outdated != 1 {
		t.Errorf("outdated: got %d want 1", outdated)
	}
	if nearLineDB == nil || *nearLineDB != 7 {
		t.Errorf("outdated_near_line: got %v want 7", nearLineDB)
	}
}

func TestWriteAnchorUpdates_NoUpdates_ColumnsRemainNull(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:52", 52, "sha-h52", "sha-b52")
	pos := 2
	w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID: "rc-3", PRID: pr.ID, Author: "carol", Body: "x",
			Kind: "review", CommitSHA: "sha-c52", Position: &pos,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
	}}) //nolint:errcheck

	// Sync with no AnchorUpdates (e.g. clone unavailable).
	w.WriteSyncResults([]*PRFetchResult{{
		PR:            makePR("github:o/r:52", 52, "sha-h52b", "sha-b52"),
		AnchorUpdates: nil, // no updates
	}}) //nolint:errcheck

	var anchoredLine *int
	db.QueryRow(`SELECT anchored_line FROM comments WHERE id = 'rc-3'`).Scan(&anchoredLine) //nolint:errcheck
	if anchoredLine != nil {
		t.Errorf("anchored_line should remain NULL when no AnchorUpdates, got %d", *anchoredLine)
	}
}

func TestWriteAnchorUpdates_ErrorSkippedSilently(t *testing.T) {
	// AnchorUpdate referencing a non-existent comment ID is a no-op (UPDATE 0 rows).
	// The writer must not fail the batch for this.
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)
	pr := makePR("github:o/r:53", 53, "sha-h53", "sha-b53")
	mustWritePR(t, w, pr)

	results := []*PRFetchResult{{
		PR: makePR("github:o/r:53", 53, "sha-h53", "sha-b53"),
		AnchorUpdates: []git.AnchorResult{{
			CommentID:    "nonexistent-comment",
			AnchoredLine: 5,
			HeadSHA:      "sha-h53",
			Anchored:     true,
		}},
	}}
	if err := w.WriteSyncResults(results); err != nil {
		t.Fatalf("WriteSyncResults must not fail for missing comment anchor: %v", err)
	}
}

// ── First-sync anchor identity tests ─────────────────────────────────────

// TestWriteAnchorUpdates_FirstSync_GetsCurrentPatchsetNum is the primary
// regression test for the "second sync needed" bug: a current-position anchor
// resolved during first sync has PatchsetNumber=0 (patchset not in DB at fetch
// time), but writeAnchorUpdates must fill it in with currentPatchsetNum so
// anchor_patchset_number is non-NULL and the handler treats the anchor as fresh
// without requiring a second sync.
func TestWriteAnchorUpdates_FirstSync_GetsCurrentPatchsetNum(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// First sync: write PR + review comment + anchor update with PatchsetNumber=0.
	pr := makePR("github:o/r:80", 80, "sha-head-80", "sha-base-80")
	pos := 3
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID:        "rc-first-sync",
			PRID:      pr.ID,
			Author:    "alice",
			Body:      "nit",
			Kind:      "review",
			CommitSHA: "sha-head-80",
			Position:  &pos,
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
		// PatchsetNumber=0: simulates first sync where patchset row didn't exist
		// during fetch (resolveAnchors returned resolvedPatchsetNum=0).
		AnchorUpdates: []git.AnchorResult{{
			CommentID:      "rc-first-sync",
			AnchoredLine:   5,
			Anchored:       true,
			HeadSHA:        "sha-head-80",
			PatchsetNumber: 0, // ← the bug: was NULL before this fix
		}},
	}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	// Verify anchor_patchset_number is NON-NULL (set to currentPatchsetNum).
	var anchorPsNum *int
	db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-first-sync'`).Scan(&anchorPsNum) //nolint:errcheck
	if anchorPsNum == nil {
		t.Fatal("anchor_patchset_number must be non-NULL on first sync (not stale)")
	}

	// Verify anchor_current_patchset_number is also set (same value for current-position).
	var anchorCurPsNum *int
	db.QueryRow(`SELECT anchor_current_patchset_number FROM comments WHERE id='rc-first-sync'`).Scan(&anchorCurPsNum) //nolint:errcheck
	if anchorCurPsNum == nil {
		t.Fatal("anchor_current_patchset_number must be non-NULL on first sync")
	}
	if *anchorCurPsNum == 0 {
		t.Error("anchor_current_patchset_number must be > 0")
	}

	// Verify the patchset row was written (for completeness).
	var psCount int
	db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id=?`, pr.ID).Scan(&psCount) //nolint:errcheck
	if psCount == 0 {
		t.Error("patchset row must exist after WriteSyncResults")
	}

	// Verify anchor_patchset_number matches the patchset that was written.
	var currentPsNum int
	db.QueryRow(`SELECT number FROM patchsets WHERE pr_id=? AND head_sha=? AND base_sha=?`,
		pr.ID, "sha-head-80", "sha-base-80",
	).Scan(&currentPsNum) //nolint:errcheck
	if *anchorPsNum != currentPsNum {
		t.Errorf("anchor_patchset_number=%d must equal currentPatchsetNum=%d", *anchorPsNum, currentPsNum)
	}
}

// TestWriteAnchorUpdates_OriginalPosition_KeepsOriginalPatchsetNum verifies
// that an original-position anchor (PatchsetNumber > 0) is NOT overwritten
// with the current patchset number, preserving its correct historical identity.
func TestWriteAnchorUpdates_OriginalPosition_KeepsOriginalPatchsetNum(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// Set up: two patchsets. Current PR is on P2; the comment was made on P1.
	pr := makePR("github:o/r:81", 81, "sha-head-81", "sha-base-81")

	// First write: record P1 (initial patchset).
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: makePR("github:o/r:81", 81, "sha-head-81a", "sha-base-81"),
		Comments: []*provider.Comment{{
			ID: "rc-orig-ps", PRID: pr.ID, Author: "bob", Body: "q",
			Kind: "review", CommitSHA: "sha-head-81a",
			Position:  func() *int { p := 2; return &p }(),
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
	}}); err != nil {
		t.Fatalf("P1 write: %v", err)
	}

	// Manually set the comment to simulate it was anchored for P1 with PatchsetNumber=1.
	db.Exec(`UPDATE comments SET patchset_number=1 WHERE id='rc-orig-ps'`) //nolint:errcheck

	// Second write: PR moves to new head (P2 = currentPatchsetNum=2).
	// The comment is now outdated (original-position), anchor was resolved for P1.
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr, // sha-head-81, sha-base-81 → P2
		AnchorUpdates: []git.AnchorResult{{
			CommentID:        "rc-orig-ps",
			Outdated:         true,
			OutdatedNearLine: 3,
			Anchored:         false,
			HeadSHA:          "sha-head-81",
			PatchsetNumber:   1, // ← original patchset, must NOT be overwritten with P2
		}},
	}}); err != nil {
		t.Fatalf("P2 write: %v", err)
	}

	// anchor_patchset_number must remain 1 (original patchset), not 2 (current).
	var anchorPsNum *int
	db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-orig-ps'`).Scan(&anchorPsNum) //nolint:errcheck
	if anchorPsNum == nil || *anchorPsNum != 1 {
		t.Errorf("original-position anchor: anchor_patchset_number should be 1 (original), got %v", anchorPsNum)
	}
}

// TestWriteAnchorUpdates_BothZero_WritesNull verifies that when both
// PatchsetNumber=0 AND currentPatchsetNum=0 (degenerate case), anchor_patchset_number
// is NULL — handler treats it as stale, which is the correct safe behavior.
func TestWriteAnchorUpdates_BothZero_WritesNull(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	// Write a PR without a patchset (edge case: recordPatchsetIfChanged failed
	// would trigger savepoint rollback, so in practice currentPatchsetNum > 0
	// when we reach writeAnchorUpdates — but test the defensive path anyway).
	pr := makePR("github:o/r:82", 82, "sha-head-82", "sha-base-82")
	mustWritePR(t, w, pr)
	db.Exec(`INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind)
		VALUES ('rc-zero-ps', ?, 'c', 'b', '2026-01-01', '2026-01-01', 'review')`, pr.ID) //nolint:errcheck

	// Directly call writeAnchorUpdates with currentPatchsetNum=0 (both zero).
	ctx := context.Background()
	tx, _ := db.BeginTx(ctx, nil)
	w.writeAnchorUpdates(ctx, tx, []git.AnchorResult{{
		CommentID:      "rc-zero-ps",
		AnchoredLine:   7,
		Anchored:       true,
		HeadSHA:        "sha-head-82",
		PatchsetNumber: 0, // fetcher didn't find patchset
	}}, 0 /* currentPatchsetNum also 0 */)
	tx.Commit() //nolint:errcheck

	var anchorPsNum *int
	db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-zero-ps'`).Scan(&anchorPsNum) //nolint:errcheck
	if anchorPsNum != nil {
		t.Errorf("both-zero: anchor_patchset_number should be NULL (stale), got %d", *anchorPsNum)
	}
}
