package writer

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/git/anchor"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store/storetest"
)

// ── Anchor write-back tests ───────────────────────────────────────────────

func TestWriteAnchorUpdates_WritesAnchoredLine(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)

	pr := makePR("github:o/r:50", 50, "sha-head", "sha-base")
	pos := 5
	// Write PR with a review comment.
	if err := w.WriteSyncResults([]*FetchedPR{{
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
	if err := w.WriteSyncResults([]*FetchedPR{{
		PR: makePR("github:o/r:50", 50, "sha-head", "sha-base"),
		AnchorUpdates: []anchor.Result{{
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
	if err := db.QueryRow(`SELECT anchored_line, anchor_head_sha, outdated FROM comments WHERE id = 'rc-1'`).
		Scan(&anchoredLine, &anchorHeadSHA, &outdated); err != nil {
		t.Fatalf("query rc-1 anchor: %v", err)
	}
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
	db := storetest.Open(t)
	w := New(nil, db)

	pr := makePR("github:o/r:51", 51, "sha-h51", "sha-b51")
	pos := 3
	if err := w.WriteSyncResults([]*FetchedPR{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID: "rc-2", PRID: pr.ID, Author: "bob", Body: "q",
			Kind: "review", CommitSHA: "sha-c51", Position: &pos,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
	}}); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	if err := w.WriteSyncResults([]*FetchedPR{{
		PR: makePR("github:o/r:51", 51, "sha-h51", "sha-b51"),
		AnchorUpdates: []anchor.Result{{
			CommentID:        "rc-2",
			HeadSHA:          "sha-h51",
			Outdated:         true,
			OutdatedNearLine: 7,
		}},
	}}); err != nil {
		t.Fatalf("anchor write: %v", err)
	}

	var anchoredLine *int
	var outdated int
	var nearLineDB *int
	if err := db.QueryRow(`SELECT anchored_line, outdated, outdated_near_line FROM comments WHERE id = 'rc-2'`).
		Scan(&anchoredLine, &outdated, &nearLineDB); err != nil {
		t.Fatalf("query rc-2 anchor: %v", err)
	}
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
	db := storetest.Open(t)
	w := New(nil, db)

	pr := makePR("github:o/r:52", 52, "sha-h52", "sha-b52")
	pos := 2
	if err := w.WriteSyncResults([]*FetchedPR{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID: "rc-3", PRID: pr.ID, Author: "carol", Body: "x",
			Kind: "review", CommitSHA: "sha-c52", Position: &pos,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}},
	}}); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	// Sync with no AnchorUpdates (e.g. clone unavailable).
	if err := w.WriteSyncResults([]*FetchedPR{{
		PR:            makePR("github:o/r:52", 52, "sha-h52b", "sha-b52"),
		AnchorUpdates: nil, // no updates
	}}); err != nil {
		t.Fatalf("sync without anchors: %v", err)
	}

	var anchoredLine *int
	if err := db.QueryRow(`SELECT anchored_line FROM comments WHERE id = 'rc-3'`).Scan(&anchoredLine); err != nil {
		t.Fatalf("query rc-3 anchor: %v", err)
	}
	if anchoredLine != nil {
		t.Errorf("anchored_line should remain NULL when no AnchorUpdates, got %d", *anchoredLine)
	}
}

func TestWriteAnchorUpdates_ErrorSkippedSilently(t *testing.T) {
	// AnchorUpdate referencing a non-existent comment ID is a no-op (UPDATE 0 rows).
	// The writer must not fail the batch for this.
	db := storetest.Open(t)
	w := New(nil, db)
	pr := makePR("github:o/r:53", 53, "sha-h53", "sha-b53")
	mustWritePR(t, w, pr)

	results := []*FetchedPR{{
		PR: makePR("github:o/r:53", 53, "sha-h53", "sha-b53"),
		AnchorUpdates: []anchor.Result{{
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
	db := storetest.Open(t)
	w := New(nil, db)

	// First sync: write PR + review comment + anchor update with PatchsetNumber=0.
	pr := makePR("github:o/r:80", 80, "sha-head-80", "sha-base-80")
	pos := 3
	if err := w.WriteSyncResults([]*FetchedPR{{
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
		AnchorUpdates: []anchor.Result{{
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
	if err := db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-first-sync'`).Scan(&anchorPsNum); err != nil {
		t.Fatalf("query source patchset: %v", err)
	}
	if anchorPsNum == nil {
		t.Fatal("anchor_patchset_number must be non-NULL on first sync (not stale)")
	}

	// Verify anchor_current_patchset_number is also set (same value for current-position).
	var anchorCurPsNum *int
	if err := db.QueryRow(`SELECT anchor_current_patchset_number FROM comments WHERE id='rc-first-sync'`).Scan(&anchorCurPsNum); err != nil {
		t.Fatalf("query current anchor patchset: %v", err)
	}
	if anchorCurPsNum == nil {
		t.Fatal("anchor_current_patchset_number must be non-NULL on first sync")
	}
	if *anchorCurPsNum == 0 {
		t.Error("anchor_current_patchset_number must be > 0")
	}

	// Verify the patchset row was written (for completeness).
	var psCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM patchsets WHERE pr_id=?`, pr.ID).Scan(&psCount); err != nil {
		t.Fatalf("count patchsets: %v", err)
	}
	if psCount == 0 {
		t.Error("patchset row must exist after WriteSyncResults")
	}

	// Verify anchor_patchset_number matches the patchset that was written.
	var currentPsNum int
	if err := db.QueryRow(`SELECT number FROM patchsets WHERE pr_id=? AND head_sha=? AND base_sha=?`,
		pr.ID, "sha-head-80", "sha-base-80",
	).Scan(&currentPsNum); err != nil {
		t.Fatalf("query recorded patchset: %v", err)
	}
	if *anchorPsNum != currentPsNum {
		t.Errorf("anchor_patchset_number=%d must equal currentPatchsetNum=%d", *anchorPsNum, currentPsNum)
	}
}

// TestWriteAnchorUpdates_OriginalPosition_KeepsOriginalPatchsetNum verifies
// that an original-position anchor (PatchsetNumber > 0) is NOT overwritten
// with the current patchset number, preserving its correct historical identity.
func TestWriteAnchorUpdates_OriginalPosition_KeepsOriginalPatchsetNum(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)

	// Set up: two patchsets. Current PR is on P2; the comment was made on P1.
	pr := makePR("github:o/r:81", 81, "sha-head-81", "sha-base-81")

	// First write: record P1 (initial patchset).
	if err := w.WriteSyncResults([]*FetchedPR{{
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
	if _, err := db.Exec(`UPDATE comments SET patchset_number=1 WHERE id='rc-orig-ps'`); err != nil {
		t.Fatalf("set original patchset: %v", err)
	}

	// Second write: PR moves to new head (P2 = currentPatchsetNum=2).
	// The comment is now outdated (original-position), anchor was resolved for P1.
	if err := w.WriteSyncResults([]*FetchedPR{{
		PR: pr, // sha-head-81, sha-base-81 → P2
		AnchorUpdates: []anchor.Result{{
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
	if err := db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-orig-ps'`).Scan(&anchorPsNum); err != nil {
		t.Fatalf("query original anchor patchset: %v", err)
	}
	if anchorPsNum == nil || *anchorPsNum != 1 {
		t.Errorf("original-position anchor: anchor_patchset_number should be 1 (original), got %v", anchorPsNum)
	}
}

// TestWriteAnchorUpdates_BothZero_WritesNull verifies that when both
// PatchsetNumber=0 AND currentPatchsetNum=0 (degenerate case), anchor_patchset_number
// is NULL — handler treats it as stale, which is the correct safe behavior.
func TestWriteAnchorUpdates_BothZero_WritesNull(t *testing.T) {
	db := storetest.Open(t)
	w := New(nil, db)

	// Write a PR without a patchset (edge case: recordPatchsetIfChanged failed
	// would trigger savepoint rollback, so in practice currentPatchsetNum > 0
	// when we reach writeAnchorUpdates — but test the defensive path anyway).
	pr := makePR("github:o/r:82", 82, "sha-head-82", "sha-base-82")
	mustWritePR(t, w, pr)
	if _, err := db.Exec(`INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, kind)
		VALUES ('rc-zero-ps', ?, 'c', 'b', '2026-01-01', '2026-01-01', 'review')`, pr.ID); err != nil {
		t.Fatalf("insert zero-patchset comment: %v", err)
	}

	// Directly call writeAnchorUpdates with currentPatchsetNum=0 (both zero).
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin anchor transaction: %v", err)
	}
	t.Cleanup(func() {
		// Rollback after Commit returns sql.ErrTxDone; that is expected.
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Fatalf("rollback anchor transaction: %v", err)
		}
	})
	w.writeAnchorUpdates(ctx, tx, []anchor.Result{{
		CommentID:      "rc-zero-ps",
		AnchoredLine:   7,
		Anchored:       true,
		HeadSHA:        "sha-head-82",
		PatchsetNumber: 0, // fetcher didn't find patchset
	}}, 0 /* currentPatchsetNum also 0 */)
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit anchor transaction: %v", err)
	}

	var anchorPsNum *int
	if err := db.QueryRow(`SELECT anchor_patchset_number FROM comments WHERE id='rc-zero-ps'`).Scan(&anchorPsNum); err != nil {
		t.Fatalf("query zero anchor patchset: %v", err)
	}
	if anchorPsNum != nil {
		t.Errorf("both-zero: anchor_patchset_number should be NULL (stale), got %d", *anchorPsNum)
	}
}
