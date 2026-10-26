package anchor

import (
	"fmt"
	"os"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ── Resolve — table-driven ─────────────────────────────────────────────────
//
// Design rules for test content:
//   - Insertions/deletions must be > contextLines (3) away from the target line
//     so the ±3 context window of the target is identical in both commits.
//   - "moved down": target near end of file; insert 1 line at the top.
//   - "moved up":  target near end of file; delete 1 line from the top.

type resolveCase struct {
	name         string
	contentA     string
	contentB     string
	commentLine  int // 1-based line in commit A
	wantAnchored bool
	wantOutdated bool
	wantLine     int // AnchoredLine or OutdatedNearLine; 0 = don't check exact value
}

var resolveCases = []resolveCase{
	{
		// Target line unchanged; contentB adds a line far from the target so
		// the ±3 context window around the target is identical in both commits.
		// File: 8 lines, target at line 5. B adds 1 line at the very end.
		name:         "content survived same line",
		contentA:     "a\nb\nc\nd\ntarget\nf\ng\nh\n",
		contentB:     "a\nb\nc\nd\ntarget\nf\ng\nh\nextra\n",
		commentLine:  5,
		wantAnchored: true, wantOutdated: false, wantLine: 5,
	},
	{
		// Insert 1 line at the top; target moves from line 5 to line 6.
		// Context of target (indices 1..7 of ["b","c","d","target","f","g","h"])
		// is the same in both commits.
		name:         "content moved down",
		contentA:     "a\nb\nc\nd\ntarget\nf\ng\nh\n",
		contentB:     "NEW\na\nb\nc\nd\ntarget\nf\ng\nh\n",
		commentLine:  5,
		wantAnchored: true, wantOutdated: false, wantLine: 6,
	},
	{
		// Delete 1 line at the top; target moves from line 6 to line 5.
		name:         "content moved up",
		contentA:     "EXTRA\na\nb\nc\nd\ntarget\nf\ng\nh\n",
		contentB:     "a\nb\nc\nd\ntarget\nf\ng\nh\n",
		commentLine:  6,
		wantAnchored: true, wantOutdated: false, wantLine: 5,
	},
	{
		// Target line deleted; surviving nearby lines → OutdatedNearLine > 0.
		name:         "content deleted",
		contentA:     "a\nb\ntarget\nd\ne\n",
		contentB:     "a\nb\nd\ne\n",
		commentLine:  3,
		wantAnchored: false, wantOutdated: true,
	},
}

func TestAnchorResolver_Resolve(t *testing.T) {
	for _, tc := range resolveCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			_, commitSHA, headSHA := buildTwoCommitRepo(t, dir, tc.contentA, tc.contentB)
			r := NewResolver()

			// Root commit: position == line number (all additions, no parent).
			pos := tc.commentLine
			res, err := r.Resolve(Request{
				LocalPath:   dir,
				CommitSHA:   commitSHA,
				HeadSHA:     headSHA,
				FilePath:    "foo.go",
				Side:        "RIGHT",
				Position:    pos,
				CommentID:   "c1",
				PatchsetNum: 1,
				CurrentNum:  2,
			})
			if err != nil && tc.wantAnchored {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Outdated != tc.wantOutdated {
				t.Errorf("Outdated: got %v want %v (err=%v)", res.Outdated, tc.wantOutdated, err)
			}
			if res.Anchored != tc.wantAnchored {
				t.Errorf("Anchored: got %v want %v (err=%v)", res.Anchored, tc.wantAnchored, err)
			}
			if tc.wantLine != 0 {
				line := res.AnchoredLine
				if tc.wantOutdated {
					line = res.OutdatedNearLine
				}
				if line != tc.wantLine {
					t.Errorf("line: got %d want %d", line, tc.wantLine)
				}
			}
			// content deleted: OutdatedNearLine must be > 0.
			if tc.name == "content deleted" && res.OutdatedNearLine == 0 {
				t.Error("content deleted: OutdatedNearLine should be > 0")
			}
		})
	}
}

func TestAnchorResolver_FileDeleted(t *testing.T) {
	dir := t.TempDir()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, _ := repo.Worktree()
	sig := &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()}

	// Commit A: foo.go with content.
	os.WriteFile(dir+"/foo.go", []byte("a\nb\ntarget\nd\ne\n"), 0644) //nolint:errcheck
	wt.Add("foo.go")                                                  //nolint:errcheck
	hashA, _ := wt.Commit("A", &gogit.CommitOptions{Author: sig})

	// Commit B: remove foo.go, add bar.go so the commit is non-empty.
	os.Remove(dir + "/foo.go")
	os.WriteFile(dir+"/bar.go", []byte("other\n"), 0644) //nolint:errcheck
	wt.Add("bar.go")                                     //nolint:errcheck
	wt.Remove("foo.go")                                  //nolint:errcheck
	hashB, _ := wt.Commit("B", &gogit.CommitOptions{Author: sig, All: true})

	r := NewResolver()
	res, _ := r.Resolve(Request{
		LocalPath:   dir,
		CommitSHA:   hashA.String(),
		HeadSHA:     hashB.String(),
		FilePath:    "foo.go",
		Side:        "RIGHT",
		Position:    3, // root commit: pos == line 3
		CommentID:   "c-deleted",
		PatchsetNum: 1,
		CurrentNum:  2,
	})
	if !res.Outdated {
		t.Error("expected Outdated=true when file deleted")
	}
	if res.Anchored {
		t.Error("expected Anchored=false when file deleted")
	}
	if res.OutdatedNearLine != 0 {
		t.Errorf("file deleted entirely: OutdatedNearLine should be 0, got %d", res.OutdatedNearLine)
	}
}

func TestAnchorResolver_SamePatchset_FastPath(t *testing.T) {
	// PatchsetNum == CurrentNum → no re-resolution; AnchoredLine = originalLine.
	dir := t.TempDir()
	_, commitSHA, headSHA := buildTwoCommitRepo(t, dir,
		"a\nb\ntarget\nd\ne\n",
		"x\ny\nz\nd\ne\n", // contentB entirely different; fast path never reads it
	)
	r := NewResolver()
	res, err := r.Resolve(Request{
		LocalPath:   dir,
		CommitSHA:   commitSHA,
		HeadSHA:     headSHA,
		FilePath:    "foo.go",
		Side:        "RIGHT",
		Position:    3,
		CommentID:   "c-same",
		PatchsetNum: 2,
		CurrentNum:  2, // same patchset: fast path
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Anchored {
		t.Error("same patchset: expected Anchored=true")
	}
	if res.AnchoredLine != 3 {
		t.Errorf("same patchset: AnchoredLine=%d want 3", res.AnchoredLine)
	}
}

func TestAnchorResolver_CloneMissing(t *testing.T) {
	r := NewResolver()
	res, err := r.Resolve(Request{
		LocalPath:   "/tmp/bistro-nonexistent-clone-99999",
		CommitSHA:   fmt.Sprintf("%040d", 1),
		HeadSHA:     fmt.Sprintf("%040d", 2),
		FilePath:    "foo.go",
		Side:        "RIGHT",
		Position:    1,
		CommentID:   "c-missing",
		PatchsetNum: 1,
		CurrentNum:  2,
	})
	if err == nil {
		t.Error("expected error when clone is missing")
	}
	if res.Anchored {
		t.Error("Anchored must be false when clone is missing")
	}
}

func TestAnchorResolver_IdenticalContentDifferentContext(t *testing.T) {
	// Two "target" lines in contentB: one at line 4 (same context as A), one at
	// line 8 (different context). Resolver must anchor to line 4.
	//
	// contentA: 7 lines, target at line 4.
	// contentB: 11 lines, target at lines 4 and 8, with different surrounding context.
	// The ±3 context window for line 4 in A and B must be the same (lines 1..7 are
	// identical in both commits around the target).
	contentA := "a\nb\nc\ntarget\nd\ne\nf\n"
	contentB := "a\nb\nc\ntarget\nd\ne\nf\ntarget\ng\nh\ni\n"
	dir := t.TempDir()
	_, commitSHA, headSHA := buildTwoCommitRepo(t, dir, contentA, contentB)
	r := NewResolver()

	res, err := r.Resolve(Request{
		LocalPath:   dir,
		CommitSHA:   commitSHA,
		HeadSHA:     headSHA,
		FilePath:    "foo.go",
		Side:        "RIGHT",
		Position:    4, // root commit: pos == line 4 ("target")
		CommentID:   "c-dup",
		PatchsetNum: 1,
		CurrentNum:  2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Anchored {
		t.Errorf("expected Anchored=true; err=%v outdated=%v nearLine=%d",
			err, res.Outdated, res.OutdatedNearLine)
	}
	// Must anchor to line 4 (same context), not line 8 (different context).
	if res.AnchoredLine != 4 {
		t.Errorf("AnchoredLine=%d want 4 (context should discriminate duplicate lines)",
			res.AnchoredLine)
	}
}

// TestResolve_WithDiffBaseSHA_CorrectGeometry verifies end-to-end that a
// Request with DiffBaseSHA produces the correct AnchoredLine using the
// patchset geometry rather than parent-of-commit geometry.
func TestResolve_WithDiffBaseSHA_CorrectGeometry(t *testing.T) {
	// 3-commit repo: A→B→C.  PR base=A, head=C.
	// Comment posted on C at pos 8 (RIGHT, A→C diff) = LINE6_CHANGED at line 7.
	// After "rebase" (changing C content): the comment should re-anchor correctly.
	//
	// For simplicity: HeadSHA=C so the same-commit fast path fires and we just
	// verify that originalLine (from A→C) = 7, not the wrong value from B→C.
	contentA := "line1\nline2\nline3\nline4\nline5\nline6\n"
	contentB := "line1\nline2\nline3\nINSERTED\nline4\nline5\nline6\n"
	contentC := "line1\nline2\nline3\nINSERTED\nline4\nline5\nLINE6_CHANGED\n"

	dir := t.TempDir()
	shaA, _, shaC := buildThreeCommitRepo(t, dir, contentA, contentB, contentC)
	r := NewResolver()

	// CommitSHA = HeadSHA = C: fast path returns originalLine directly.
	// This lets us verify the position→line mapping without needing the hash scan.
	res, err := r.Resolve(Request{
		LocalPath:   dir,
		CommitSHA:   shaC,
		HeadSHA:     shaC, // same commit → fast path
		DiffBaseSHA: shaA, // CORRECT: PR base is A
		FilePath:    "foo.go",
		Side:        "RIGHT",
		Position:    8, // A→C diff pos 8 = LINE6_CHANGED at new line 7
		CommentID:   "c-geom",
		PatchsetNum: 1,
		CurrentNum:  -1,
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !res.Anchored {
		t.Error("expected Anchored=true (same-commit fast path)")
	}
	if res.AnchoredLine != 7 {
		t.Errorf("AnchoredLine: got %d, want 7 (LINE6_CHANGED is at line 7 in C)", res.AnchoredLine)
	}
}
