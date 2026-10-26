package sync

import (
	"testing"

	"github.com/itterno/bistro/internal/provider"
)

// TestWriteComment_OutdatedComment_PreservesPatchsetNumber verifies requirement 4:
// outdated/rebased comments (Position==nil, OriginalPosition set) must NOT have
// their patchset_number overwritten with the current patchset number.
// The COALESCE in writeComment preserves the original value when patchsetNum==0.
func TestWriteComment_OutdatedComment_PreservesPatchsetNumber(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:60", 60, "head-60", "base-60")
	origPos := 5

	// First sync: write the comment with patchset_number=1 (original patchset).
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID:        "outdated-c1",
			PRID:      pr.ID,
			Author:    "alice",
			Body:      "review",
			Kind:      "review",
			CommitSHA: "sha-orig",
			Position:  &origPos, // has current position on first write
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
	}}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// Manually set patchset_number=1 to simulate it was written on P1.
	db.Exec(`UPDATE comments SET patchset_number=1 WHERE id='outdated-c1'`) //nolint:errcheck

	// Second sync: comment is now outdated (Position==nil), OriginalPosition set.
	// This represents a force-push after which GitHub marks the comment outdated.
	newPR := makePR("github:o/r:60", 60, "head-60b", "base-60") // new head → new patchset
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: newPR,
		Comments: []*provider.Comment{{
			ID:               "outdated-c1",
			PRID:             pr.ID,
			Author:           "alice",
			Body:             "review",
			Kind:             "review",
			CommitSHA:        "", // cleared by GitHub for outdated comments
			OriginalSHA:      "sha-orig",
			Position:         nil,      // no current position — outdated
			OriginalPosition: &origPos, // original position still set
			CreatedAt:        mustTime(), UpdatedAt: mustTime(),
		}},
	}}); err != nil {
		t.Fatalf("second write (outdated): %v", err)
	}

	// patchset_number must still be 1 (NOT overwritten with current patchset number).
	var psNum *int
	db.QueryRow(`SELECT patchset_number FROM comments WHERE id='outdated-c1'`).Scan(&psNum) //nolint:errcheck
	if psNum == nil || *psNum != 1 {
		t.Errorf("outdated comment patchset_number: got %v want 1 (must not be overwritten with current patchset)", psNum)
	}
}

func TestWriteComment_ReviewPositionFields(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:61", 61, "head-61", "base-61")
	pos := 3
	origPos := 4
	origLine := 40
	startLine := 38
	origStartLine := 37
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Comments: []*provider.Comment{{
			ID: "review-position-c1", PRID: pr.ID, Author: "alice", Body: "range nit",
			Kind: "review", CommitSHA: "head-61", OriginalSHA: "orig-61", Side: "RIGHT",
			StartSide: "LEFT", Position: &pos, OriginalPosition: &origPos,
			OriginalLine: &origLine, StartLine: &startLine, OriginalStartLine: &origStartLine,
			SubjectType: "LINE", FilePath: "main.go", LineNo: 41,
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
	}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	var gotOriginalLine, gotStartLine, gotOriginalStartLine *int
	var gotStartSide, gotSubjectType *string
	if err := db.QueryRow(`SELECT original_line, start_line, original_start_line, start_side, subject_type FROM comments WHERE id='review-position-c1'`).Scan(
		&gotOriginalLine, &gotStartLine, &gotOriginalStartLine, &gotStartSide, &gotSubjectType,
	); err != nil {
		t.Fatalf("query review positioning fields: %v", err)
	}
	if gotOriginalLine == nil || *gotOriginalLine != 40 || gotStartLine == nil || *gotStartLine != 38 || gotOriginalStartLine == nil || *gotOriginalStartLine != 37 {
		t.Fatalf("line positioning fields = original:%v start:%v original_start:%v", gotOriginalLine, gotStartLine, gotOriginalStartLine)
	}
	if gotStartSide == nil || *gotStartSide != "LEFT" || gotSubjectType == nil || *gotSubjectType != "LINE" {
		t.Fatalf("side/subject fields = start_side:%v subject_type:%v", gotStartSide, gotSubjectType)
	}
}

// ── Blocker 1: thread comment patchset_number stamping ────────────────────

// TestWriteThreadComment_CurrentPosition_StampsPatchsetNumber verifies that
// review comments arriving via result.Threads (not result.Comments) receive
// the current patchset_number when they have a current Position.
func TestWriteThreadComment_CurrentPosition_StampsPatchsetNumber(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:70", 70, "head-70", "base-70")
	pos := 3
	thread := &provider.ReviewThread{
		ID:   "thread-70",
		PRID: pr.ID,
		Comments: []*provider.Comment{{
			ID:        "tc-current",
			PRID:      pr.ID,
			Author:    "alice",
			Body:      "thread comment",
			Kind:      "review",
			CommitSHA: "head-70",
			Position:  &pos, // current position → should get patchset_number
			CreatedAt: mustTime(), UpdatedAt: mustTime(),
		}},
	}

	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR:      pr,
		Threads: []*provider.ReviewThread{thread},
	}}); err != nil {
		t.Fatalf("WriteSyncResults: %v", err)
	}

	var psNum *int
	db.QueryRow(`SELECT patchset_number FROM comments WHERE id='tc-current'`).Scan(&psNum) //nolint:errcheck
	// patchset was written by the PR write; thread comment should have been stamped.
	if psNum == nil || *psNum == 0 {
		t.Errorf("thread comment with current position: patchset_number should be stamped, got %v", psNum)
	}
}

// TestWriteThreadComment_OutdatedPosition_PreservesPatchsetNumber verifies that
// an outdated thread comment (Position==nil) preserves its existing patchset_number
// instead of being overwritten with the current patchset number.
func TestWriteThreadComment_OutdatedPosition_PreservesPatchsetNumber(t *testing.T) {
	db := newSyncTestDB(t)
	w := NewWriter(nil, db)

	pr := makePR("github:o/r:71", 71, "head-71", "base-71")
	origPos := 5

	// First sync: current comment with patchset stamped.
	pos := 5
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: pr,
		Threads: []*provider.ReviewThread{{
			ID:   "thread-71",
			PRID: pr.ID,
			Comments: []*provider.Comment{{
				ID:        "tc-outdated",
				PRID:      pr.ID,
				Author:    "bob",
				Body:      "thread comment",
				Kind:      "review",
				CommitSHA: "head-71",
				Position:  &pos,
				CreatedAt: mustTime(), UpdatedAt: mustTime(),
			}},
		}},
	}}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// Manually mark patchset_number=1 (as if it was stamped on first sync).
	db.Exec(`UPDATE comments SET patchset_number=1 WHERE id='tc-outdated'`) //nolint:errcheck

	// Second sync: the thread comment is now outdated (Position==nil after force-push).
	newPR := makePR("github:o/r:71", 71, "head-71b", "base-71")
	if err := w.WriteSyncResults([]*PRFetchResult{{
		PR: newPR,
		Threads: []*provider.ReviewThread{{
			ID:   "thread-71",
			PRID: pr.ID,
			Comments: []*provider.Comment{{
				ID:               "tc-outdated",
				PRID:             pr.ID,
				Author:           "bob",
				Body:             "thread comment",
				Kind:             "review",
				OriginalSHA:      "head-71",
				Position:         nil, // outdated — no current position
				OriginalPosition: &origPos,
				CreatedAt:        mustTime(), UpdatedAt: mustTime(),
			}},
		}},
	}}); err != nil {
		t.Fatalf("second write: %v", err)
	}

	var psNum *int
	db.QueryRow(`SELECT patchset_number FROM comments WHERE id='tc-outdated'`).Scan(&psNum) //nolint:errcheck
	if psNum == nil || *psNum != 1 {
		t.Errorf("outdated thread comment: patchset_number should remain 1, got %v", psNum)
	}
}
