package anchor

import "testing"

// ── FileLineHash ───────────────────────────────────────────────────────────

func TestFileLineHash_Basic(t *testing.T) {
	dir := t.TempDir()
	content := "alpha\nbeta\ngamma\ndelta\nepsilon\n"
	_, sha_A, _ := buildTwoCommitRepo(t, dir, content, content+"zeta\n")
	r := NewResolver()

	lh, ch, err := r.FileLineHash(dir, sha_A, "foo.go", 3) // "gamma"
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lh == "" || ch == "" {
		t.Error("expected non-empty hashes")
	}

	// Deterministic.
	lh2, ch2, _ := r.FileLineHash(dir, sha_A, "foo.go", 3)
	if lh != lh2 || ch != ch2 {
		t.Error("FileLineHash is not deterministic")
	}
}

func TestFileLineHash_ContextWindow(t *testing.T) {
	// Two files with identical "target" line but different context → different ctxHash.
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	content1 := "a\nb\ntarget\nd\ne\n"
	content1b := "a\nb\ntarget\nd\ne\nf\n" // slightly different B
	content2 := "x\ny\ntarget\nw\nz\n"
	content2b := "x\ny\ntarget\nw\nz\nv\n"
	_, sha1, _ := buildTwoCommitRepo(t, dir1, content1, content1b)
	_, sha2, _ := buildTwoCommitRepo(t, dir2, content2, content2b)
	r := NewResolver()

	lh1, ch1, _ := r.FileLineHash(dir1, sha1, "foo.go", 3)
	lh2, ch2, _ := r.FileLineHash(dir2, sha2, "foo.go", 3)
	if lh1 != lh2 {
		t.Error("line hashes should be identical for same content 'target'")
	}
	if ch1 == ch2 {
		t.Error("context hashes should differ when surrounding lines differ")
	}
}
