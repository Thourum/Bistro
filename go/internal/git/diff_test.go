package git

import (
	"testing"
)

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
