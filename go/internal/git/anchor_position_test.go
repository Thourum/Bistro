package git

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// ── fixture helper ─────────────────────────────────────────────────────────

// buildTwoCommitRepo initialises an on-disk repo at dir and creates two
// commits for "foo.go": commit A with contentA, commit B with contentB.
// Returns (dir, sha_A, sha_B). contentB must differ from contentA.
func buildTwoCommitRepo(t *testing.T, dir, contentA, contentB string) (string, string, string) {
	t.Helper()
	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}

	writeAndCommit := func(content, msg string) plumbing.Hash {
		if err := os.WriteFile(dir+"/foo.go", []byte(content), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := wt.Add("foo.go"); err != nil {
			t.Fatalf("Add: %v", err)
		}
		h, err := wt.Commit(msg, &gogit.CommitOptions{Author: sig})
		if err != nil {
			t.Fatalf("Commit %q: %v", msg, err)
		}
		return h
	}

	hashA := writeAndCommit(contentA, "commit A")
	hashB := writeAndCommit(contentB, "commit B")
	return dir, hashA.String(), hashB.String()
}

// ── PositionToLine ─────────────────────────────────────────────────────────

func TestPositionToLine_RootCommit(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\nline4\nline5\n"
	// commit B slightly different (must avoid empty-commit error)
	_, sha_A, _ := buildTwoCommitRepo(t, dir, content, content+"extra\n")
	r := NewAnchorResolver()

	// Root commit: all lines are additions; position == line number.
	for wantLine := 1; wantLine <= 5; wantLine++ {
		got, err := r.PositionToLine(dir, plumbing.ZeroHash.String(), sha_A, "foo.go", "RIGHT", wantLine)
		if err != nil {
			t.Fatalf("line %d: %v", wantLine, err)
		}
		if got != wantLine {
			t.Errorf("line %d: got %d", wantLine, got)
		}
	}
}

func TestPositionToLine_ChangedLine(t *testing.T) {
	// "line4" → "LINE4_CHANGED": the diff has context+delete+add.
	dir := t.TempDir()
	_, baseSHA, headSHA := buildTwoCommitRepo(t,
		dir,
		"line1\nline2\nline3\nline4\nline5\n",
		"line1\nline2\nline3\nLINE4_CHANGED\nline5\n",
	)
	r := NewAnchorResolver()

	// The diff baseSHA..headSHA for 5-line file changing line4:
	// 3 context lines (1,2,3), 1 delete (line4), 1 add (LINE4_CHANGED), 1 context (line5)
	// LEFT pos 4 = deleted "line4" → old file line 4
	got, err := r.PositionToLine(dir, baseSHA, headSHA, "foo.go", "LEFT", 4)
	if err != nil {
		t.Fatalf("LEFT pos 4: %v", err)
	}
	if got != 4 {
		t.Errorf("LEFT pos 4: got %d want 4", got)
	}

	// RIGHT pos 5 = added "LINE4_CHANGED" → new file line 4
	got, err = r.PositionToLine(dir, baseSHA, headSHA, "foo.go", "RIGHT", 5)
	if err != nil {
		t.Fatalf("RIGHT pos 5: %v", err)
	}
	if got != 4 {
		t.Errorf("RIGHT pos 5: got %d want 4", got)
	}
}

func TestPositionToLine_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	_, sha_A, _ := buildTwoCommitRepo(t, dir, "a\nb\nc\n", "a\nb\nc\nd\n")
	r := NewAnchorResolver()
	_, err := r.PositionToLine(dir, plumbing.ZeroHash.String(), sha_A, "foo.go", "RIGHT", 9999)
	if err == nil {
		t.Error("expected error for out-of-range position")
	}
}

// ── Blocker regression tests ──────────────────────────────────────────────

// TestPositionToLine_HunkFarFromStart is the primary regression test for the
// hunk-offset bug: go-git's raw Chunks() counts from line 1, but GitHub
// positions count from the first context line of each hunk. For a 20-line file
// with a change at line 15, GitHub position 4 = deleted line 15 (after 3
// context lines), but the raw-chunks approach returned 4 (line 4 of the file).
func TestPositionToLine_HunkFarFromStart(t *testing.T) {
	// Build a 20-line file with a change at line 15.
	var orig, changed strings.Builder
	for i := 1; i <= 20; i++ {
		orig.WriteString(fmt.Sprintf("line%02d\n", i))
		if i == 15 {
			changed.WriteString("LINE15_CHANGED\n")
		} else {
			changed.WriteString(fmt.Sprintf("line%02d\n", i))
		}
	}
	dir := t.TempDir()
	_, baseSHA, headSHA := buildTwoCommitRepo(t, dir, orig.String(), changed.String())
	r := NewAnchorResolver()

	// The hunk rendered by go-git with 3 context lines:
	//   @@ -12,7 +12,7 @@
	//    line12   pos 1
	//    line13   pos 2
	//    line14   pos 3
	//   -line15   pos 4  ← deleted, old file line 15
	//   +LINE15_CHANGED pos 5
	//    line16   pos 6
	//    line17   pos 7
	//
	// Old (raw-chunks) approach: counted from line 1, so pos 4 → line 4. WRONG.
	// New (render+parse) approach: hunk header @@ -12,... sets oldLine=12-1=11,
	// so the 4th diff line is oldLine=12+3=15. CORRECT.

	got, err := r.PositionToLine(dir, baseSHA, headSHA, "foo.go", "LEFT", 4)
	if err != nil {
		t.Fatalf("PositionToLine LEFT pos 4: %v", err)
	}
	if got != 15 {
		t.Errorf("hunk-far-from-start: LEFT pos 4 → got line %d, want 15", got)
	}

	// Position 5 = added LINE15_CHANGED → new file line 15.
	got, err = r.PositionToLine(dir, baseSHA, headSHA, "foo.go", "RIGHT", 5)
	if err != nil {
		t.Fatalf("PositionToLine RIGHT pos 5: %v", err)
	}
	if got != 15 {
		t.Errorf("hunk-far-from-start: RIGHT pos 5 → got line %d, want 15", got)
	}
}

// buildThreeCommitRepo creates a repo with three commits on "foo.go":
//
//	A → B → C
//
// Returns (sha_A, sha_B, sha_C).
func buildThreeCommitRepo(t *testing.T, dir, contentA, contentB, contentC string) (string, string, string) {
	t.Helper()
	// Build A→B with existing helper.
	_, shaA, shaB := buildTwoCommitRepo(t, dir, contentA, contentB)

	// Add commit C: overwrite foo.go with contentC.
	repo, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen for C: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if err := os.WriteFile(dir+"/foo.go", []byte(contentC), 0644); err != nil {
		t.Fatalf("WriteFile C: %v", err)
	}
	if _, err := wt.Add("foo.go"); err != nil {
		t.Fatalf("Add C: %v", err)
	}
	shaC, err := wt.Commit("commit C", &gogit.CommitOptions{
		Author: &object.Signature{Name: "test", Email: "t@t.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Commit C: %v", err)
	}
	return shaA, shaB, shaC.String()
}

// TestPositionToLine_MultiCommitPR_CorrectGeometry verifies that DiffBaseSHA
// produces the correct result for a 3-commit PR (A→B→C) where parent(C)=B but
// the PR base is A. Using parent(C)=B as base would give the wrong position
// mapping for a comment on the A→C diff.
func TestPositionToLine_MultiCommitPR_CorrectGeometry(t *testing.T) {
	// A: 6 lines             B: inserts "INSERTED" between line3 and line4
	// C: renames line6 to "LINE6_CHANGED"
	//
	// PR: base=A, head=C.  Review comment on the A→C diff.
	//
	// A→C diff:
	//   @@ -1,6 +1,7 @@
	//    line1            pos 1
	//    line2            pos 2
	//    line3            pos 3
	//   +INSERTED         pos 4  ← new line 4 in C
	//    line4            pos 5
	//    line5            pos 6
	//   -line6            pos 7  ← deleted, old line 6 in A
	//   +LINE6_CHANGED    pos 8  ← new line 7 in C
	//
	// B→C diff (wrong geometry):
	//   @@ -4,4 +4,4 @@
	//    line4            pos 1
	//    line5            pos 2
	//   -line6            pos 3  ← different position for same logical change
	//   +LINE6_CHANGED    pos 4

	contentA := "line1\nline2\nline3\nline4\nline5\nline6\n"
	contentB := "line1\nline2\nline3\nINSERTED\nline4\nline5\nline6\n"
	contentC := "line1\nline2\nline3\nINSERTED\nline4\nline5\nLINE6_CHANGED\n"

	dir := t.TempDir()
	shaA, _, shaC := buildThreeCommitRepo(t, dir, contentA, contentB, contentC)
	r := NewAnchorResolver()

	// With correct geometry (DiffBaseSHA=A): pos 4 RIGHT = new line 4 in C ("INSERTED").
	t.Run("correct_base_A_pos4_right", func(t *testing.T) {
		got, err := r.PositionToLine(dir, shaA, shaC, "foo.go", "RIGHT", 4)
		if err != nil {
			t.Fatalf("PositionToLine: %v", err)
		}
		// Line 4 in C is "INSERTED".
		if got != 4 {
			t.Errorf("A→C RIGHT pos 4: got line %d, want 4 ('INSERTED')", got)
		}
	})

	// With correct geometry (DiffBaseSHA=A): pos 7 LEFT = old line 6 in A ("line6").
	t.Run("correct_base_A_pos7_left", func(t *testing.T) {
		got, err := r.PositionToLine(dir, shaA, shaC, "foo.go", "LEFT", 7)
		if err != nil {
			t.Fatalf("PositionToLine: %v", err)
		}
		// Line 6 in A is "line6".
		if got != 6 {
			t.Errorf("A→C LEFT pos 7: got line %d, want 6 ('line6')", got)
		}
	})
}

// TestParseHunkHeader covers various hunk header formats.
func TestParseHunkHeader(t *testing.T) {
	cases := []struct {
		header   string
		wantOld  int
		wantNew  int
		wantFail bool
	}{
		{"@@ -47,6 +47,6 @@", 47, 47, false},
		{"@@ -1,5 +1,5 @@", 1, 1, false},
		{"@@ -0,0 +1,3 @@", 0, 1, false}, // new file
		{"@@ -12,7 +12,7 @@ func foo() {", 12, 12, false},
		{"@@ -3 +3 @@", 3, 3, false}, // no count
		{"not a hunk header", 0, 0, true},
	}
	for _, tc := range cases {
		old, new, err := parseHunkHeader(tc.header)
		if tc.wantFail {
			if err == nil {
				t.Errorf("parseHunkHeader(%q): expected error, got old=%d new=%d", tc.header, old, new)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseHunkHeader(%q): unexpected error: %v", tc.header, err)
			continue
		}
		if old != tc.wantOld || new != tc.wantNew {
			t.Errorf("parseHunkHeader(%q): got old=%d new=%d, want old=%d new=%d",
				tc.header, old, new, tc.wantOld, tc.wantNew)
		}
	}
}
