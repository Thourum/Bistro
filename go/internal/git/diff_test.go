package git

import (
	"testing"

	gogit "github.com/go-git/go-git/v5"
)

// TestGenerateDiffOnBareClone proves GenerateDiff works against a bare
// (no-checkout) clone — the storage mode Clone now uses. It builds a source
// repo with two commits, bare-clones it locally, then diffs the two commits.
func TestGenerateDiffOnBareClone(t *testing.T) {
	srcDir := t.TempDir()
	contentA := "line1\nline2\nline3\n"
	contentB := "line1\nCHANGED\nline3\nline4\n"
	_, baseSHA, headSHA := buildTwoCommitRepo(t, srcDir, contentA, contentB)

	// Bare clone the source repo into a fresh dir (no working tree).
	bareDir := t.TempDir()
	if _, err := gogit.PlainClone(bareDir, true, &gogit.CloneOptions{
		URL: srcDir,
	}); err != nil {
		t.Fatalf("bare PlainClone: %v", err)
	}

	client := NewClient(NewCache())
	result, err := client.GenerateDiff(bareDir, baseSHA, headSHA)
	if err != nil {
		t.Fatalf("GenerateDiff on bare clone: %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 changed file, got %d", len(result.Files))
	}
	f := result.Files[0]
	if f.Path != "foo.go" {
		t.Errorf("expected path foo.go, got %s", f.Path)
	}
	if f.Status != "modified" {
		t.Errorf("expected status modified, got %s", f.Status)
	}
	if f.AddedLines == 0 || f.RemovedLines == 0 {
		t.Errorf("expected added and removed lines, got +%d -%d", f.AddedLines, f.RemovedLines)
	}
	if result.Size == 0 {
		t.Error("expected non-zero diff size")
	}
}

func TestCountLines(t *testing.T) {
	patch := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main
+
 func main() {
-	fmt.Println("hello")
+	fmt.Println("world")
`

	added, removed := countLines(patch)
	if added < 1 {
		t.Errorf("expected added lines, got %d", added)
	}
	if removed < 1 {
		t.Errorf("expected removed lines, got %d", removed)
	}
}

func TestCountLinesEmpty(t *testing.T) {
	added, removed := countLines("")
	if added != 0 || removed != 0 {
		t.Errorf("expected 0,0 for empty patch, got %d,%d", added, removed)
	}
}

func TestDiffFileCreation(t *testing.T) {
	file := &DiffFile{
		Path:         "main.go",
		Status:       "modified",
		Patch:        "patch content",
		AddedLines:   5,
		RemovedLines: 3,
	}

	if file.Path != "main.go" {
		t.Errorf("expected path main.go, got %s", file.Path)
	}
	if file.Status != "modified" {
		t.Errorf("expected status modified, got %s", file.Status)
	}
}

func TestDiffResultCreation(t *testing.T) {
	result := &DiffResult{
		Files: []*DiffFile{
			{Path: "file1.go", Status: "added"},
			{Path: "file2.go", Status: "modified"},
		},
		Size: 1024,
	}

	if len(result.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(result.Files))
	}
	if result.Size != 1024 {
		t.Errorf("expected size 1024, got %d", result.Size)
	}
}
