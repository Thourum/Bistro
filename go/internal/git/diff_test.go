package git

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitlib "github.com/go-git/go-git/v5"
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
	if _, err := gitlib.PlainClone(bareDir, true, &gitlib.CloneOptions{
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
	file.OldPath = "old-main.go"
	file.Binary = true
	file.Truncated = true
	file.PatchBytes = len(file.Patch)
	file.HunkCount = 1
	if !file.Binary || !file.Truncated || file.OldPath != "old-main.go" || file.PatchBytes != len(file.Patch) || file.HunkCount != 1 {
		t.Errorf("metadata not retained: %+v", file)
	}
}

func TestDiffResultCreation(t *testing.T) {
	result := &DiffResult{
		Files: []*DiffFile{
			{Path: "file1.go", Status: "added"},
			{Path: "file2.go", Status: "modified"},
		},
		Size:    1024,
		Profile: DiffProfile{FileCount: 2, PatchBytes: 1024},
	}

	if len(result.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(result.Files))
	}
	if result.Size != 1024 {
		t.Errorf("expected size 1024, got %d", result.Size)
	}
	if result.Profile.FileCount != 2 || result.Profile.PatchBytes != 1024 {
		t.Errorf("unexpected profile counts: %+v", result.Profile)
	}
}

func TestGenerateDiffProfileMetrics(t *testing.T) {
	repoPath, baseSHA, headSHA := makeDiffTestRepo(t, map[string][]byte{
		"src/app.go": []byte("package main\n\nfunc main() {}\n"),
	}, map[string][]byte{
		"src/app.go": []byte("package main\n\nfunc main() { println(\"hi\") }\n"),
		"src/new.go": []byte("package main\n"),
	})

	result, err := NewClient(NewCache()).GenerateDiff(repoPath, baseSHA, headSHA, DiffOptions{})
	if err != nil {
		t.Fatalf("GenerateDiff: %v", err)
	}

	if result.Profile.FileCount != len(result.Files) {
		t.Fatalf("profile file count=%d, files=%d", result.Profile.FileCount, len(result.Files))
	}
	if result.Profile.PatchBytes != result.Size {
		t.Fatalf("profile patch bytes=%d, size=%d", result.Profile.PatchBytes, result.Size)
	}
	if result.Profile.TotalDuration <= 0 {
		t.Fatalf("expected positive total duration, got %s", result.Profile.TotalDuration)
	}
	if result.Profile.BuildPatch < 0 || result.Profile.RenderFilePatches < 0 {
		t.Fatalf("profile durations must be non-negative: %+v", result.Profile)
	}
}

func TestGenerateDiffForcePushHeadRecomputesPatchBytes(t *testing.T) {
	dir := t.TempDir()
	repo, worktree := initDiffTestRepo(t, dir)
	writeDiffTestFiles(t, dir, map[string][]byte{"app.txt": []byte("base\n")})
	baseSHA := commitDiffTestRepo(t, worktree, "base")

	writeDiffTestFiles(t, dir, map[string][]byte{"app.txt": []byte("base\nsmall\n")})
	headOneSHA := commitDiffTestRepo(t, worktree, "head one")

	writeDiffTestFiles(t, dir, map[string][]byte{"app.txt": []byte("base\nsmall\nforce-pushed replacement with more bytes\n")})
	headTwoSHA := commitDiffTestRepo(t, worktree, "head two")
	_ = repo

	client := NewClient(NewCache())
	first, err := client.GenerateDiff(dir, baseSHA, headOneSHA, DiffOptions{})
	if err != nil {
		t.Fatalf("GenerateDiff first head: %v", err)
	}
	second, err := client.GenerateDiff(dir, baseSHA, headTwoSHA, DiffOptions{})
	if err != nil {
		t.Fatalf("GenerateDiff force-pushed head: %v", err)
	}

	if first.Files[0].Patch == second.Files[0].Patch {
		t.Fatal("force-pushed head recomputed to identical patch; expected new head content")
	}
	if first.Profile.PatchBytes == second.Profile.PatchBytes {
		t.Fatalf("expected patch byte metric to change across heads, got %d", first.Profile.PatchBytes)
	}
}

func TestGenerateDiffBinaryFileProfilesWithoutFailure(t *testing.T) {
	repoPath, baseSHA, headSHA := makeDiffTestRepo(t, map[string][]byte{
		"assets/blob.bin": binaryBytes(16, 0),
	}, map[string][]byte{
		"assets/blob.bin": binaryBytes(16, 1),
	})

	result, err := NewClient(NewCache()).GenerateDiff(repoPath, baseSHA, headSHA, DiffOptions{})
	if err != nil {
		t.Fatalf("GenerateDiff binary file: %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected one binary diff file, got %d", len(result.Files))
	}
	if result.Profile.BinaryFileCount != 1 {
		t.Fatalf("expected binary file to be profiled, got %+v", result.Profile)
	}
	if !result.Files[0].Binary {
		t.Fatalf("expected binary metadata on diff file: %+v", result.Files[0])
	}
	if result.Files[0].PatchBytes == 0 || result.Files[0].Patch != "" {
		t.Fatalf("binary patch should retain original patch byte metadata without storing patch content: %+v", result.Files[0])
	}
	if result.Files[0].Status != "binary" {
		t.Fatalf("binary file status=%q, want binary", result.Files[0].Status)
	}
	if result.Files[0].DiffStatus != DiffStatusOK {
		t.Fatalf("binary file DiffStatus=%q, want %q", result.Files[0].DiffStatus, DiffStatusOK)
	}
	if result.Files[0].Patch != "" || result.Size != 0 || result.Profile.PatchBytes != 0 {
		t.Fatalf("binary file should store no patch bytes: file=%+v size=%d profile=%+v", result.Files[0], result.Size, result.Profile)
	}
}

func makeDiffTestRepo(t *testing.T, baseFiles, headFiles map[string][]byte) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	_, worktree := initDiffTestRepo(t, dir)
	writeDiffTestFiles(t, dir, baseFiles)
	baseSHA := commitDiffTestRepo(t, worktree, "base")
	writeDiffTestFiles(t, dir, headFiles)
	headSHA := commitDiffTestRepo(t, worktree, "head")
	return dir, baseSHA, headSHA
}

func initDiffTestRepo(t *testing.T, dir string) (*gitlib.Repository, *gitlib.Worktree) {
	t.Helper()
	repo, err := gitlib.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		t.Fatalf("open worktree: %v", err)
	}
	return repo, worktree
}

func writeDiffTestFiles(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for relPath, body := range files {
		path := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", relPath, err)
		}
		if err := os.WriteFile(path, body, 0644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}
}

func commitDiffTestRepo(t *testing.T, worktree *gitlib.Worktree, message string) string {
	t.Helper()
	if _, err := worktree.Add("."); err != nil {
		t.Fatalf("add files: %v", err)
	}
	hash, err := worktree.Commit(message, &gitlib.CommitOptions{Author: benchmarkSignature()})
	if err != nil {
		t.Fatalf("commit %q: %v", message, err)
	}
	return hash.String()
}

func binaryBytes(size, seed int) []byte {
	var b bytes.Buffer
	for i := 0; i < size; i++ {
		b.WriteByte(byte((i + seed) % 256))
	}
	return b.Bytes()
}

func TestGenerateDiffProfilesRenameShape(t *testing.T) {
	repoPath, baseSHA, headSHA := makeDiffTestRepo(t, map[string][]byte{
		"old_name.go": []byte("package main\n"),
	}, map[string][]byte{
		"old_name.go": []byte("package main\n"),
		"new_name.go": []byte("package main\n// renamed copy\n"),
	})

	result, err := NewClient(NewCache()).GenerateDiff(repoPath, baseSHA, headSHA, DiffOptions{})
	if err != nil {
		t.Fatalf("GenerateDiff rename-shaped change: %v", err)
	}
	joined := strings.Builder{}
	for _, file := range result.Files {
		fmt.Fprintf(&joined, "%s:%s\n", file.Status, file.Path)
	}
	if !strings.Contains(joined.String(), "new_name.go") {
		t.Fatalf("expected new rename path in diff files, got %s", joined.String())
	}
	if result.Profile.FileCount != len(result.Files) {
		t.Fatalf("rename profile file count mismatch: %+v files=%d", result.Profile, len(result.Files))
	}
}

func TestGenerateDiff_PerFileBudget(t *testing.T) {
	// Create a repo with one file that will generate a patch > 100 bytes
	repoPath, baseSHA, headSHA := makeDiffTestRepo(t,
		map[string][]byte{"big.go": []byte("package main\n")},
		map[string][]byte{"big.go": []byte("package main\n" + strings.Repeat("// line\n", 20))},
	)

	opts := DiffOptions{MaxFilePatchBytes: 50, MaxTotalPatchBytes: 0}
	result, err := NewClient(NewCache()).GenerateDiff(repoPath, baseSHA, headSHA, opts)
	if err != nil {
		t.Fatalf("GenerateDiff: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatal("expected at least one file in manifest")
	}
	found := false
	for _, f := range result.Files {
		if f.Path == "big.go" {
			found = true
			if f.DiffStatus != DiffStatusTruncated {
				t.Errorf("expected DiffStatus %q, got %q", DiffStatusTruncated, f.DiffStatus)
			}
			if f.Patch != "" {
				t.Errorf("expected empty patch for truncated file, got %d bytes", len(f.Patch))
			}
			if f.PatchBytes <= 50 {
				t.Errorf("expected patch_bytes to retain original size above limit, got %d", f.PatchBytes)
			}
			if f.HunkCount == 0 {
				t.Error("expected hunk_count to retain original hunk count for truncated file")
			}
		}
	}
	if !found {
		t.Error("big.go not in manifest")
	}
	if result.Profile.TruncatedFiles == 0 {
		t.Error("expected TruncatedFiles > 0 in profile")
	}
}

func TestGenerateDiff_PRBudget(t *testing.T) {
	// Two files; PR budget is very small so the second file gets too_large
	repoPath, baseSHA, headSHA := makeDiffTestRepo(t,
		map[string][]byte{
			"a.go": []byte("package main\n"),
			"b.go": []byte("package main\n"),
		},
		map[string][]byte{
			"a.go": []byte("package main\n" + strings.Repeat("// line\n", 30)),
			"b.go": []byte("package main\n" + strings.Repeat("// line\n", 30)),
		},
	)

	// per-file limit large enough to pass each file alone, but PR budget tiny
	opts := DiffOptions{MaxFilePatchBytes: 0, MaxTotalPatchBytes: 50}
	result, err := NewClient(NewCache()).GenerateDiff(repoPath, baseSHA, headSHA, opts)
	if err != nil {
		t.Fatalf("GenerateDiff: %v", err)
	}
	if len(result.Files) < 2 {
		t.Fatalf("expected manifest for both files, got %d", len(result.Files))
	}
	// At least one file must be too_large (PR budget exhausted)
	var tooLarge int
	for _, f := range result.Files {
		if f.DiffStatus == DiffStatusTooLarge {
			tooLarge++
			if f.Patch != "" {
				t.Errorf("expected empty patch for too_large file %s", f.Path)
			}
			if f.PatchBytes <= 50 {
				t.Errorf("expected patch_bytes to retain original size above PR budget for %s, got %d", f.Path, f.PatchBytes)
			}
		}
	}
	if tooLarge == 0 {
		t.Error("expected at least one too_large file when PR budget exhausted")
	}
	if result.Profile.TooLargeFiles == 0 {
		t.Error("expected TooLargeFiles > 0 in profile")
	}
}

func TestGenerateDiff_NoBudget(t *testing.T) {
	// Zero limits = no budget enforcement; existing behavior preserved
	repoPath, baseSHA, headSHA := makeDiffTestRepo(t,
		map[string][]byte{"c.go": []byte("package main\n")},
		map[string][]byte{"c.go": []byte("package main\n// added\n")},
	)
	opts := DiffOptions{} // zero = unlimited
	result, err := NewClient(NewCache()).GenerateDiff(repoPath, baseSHA, headSHA, opts)
	if err != nil {
		t.Fatalf("GenerateDiff: %v", err)
	}
	for _, f := range result.Files {
		if f.DiffStatus != DiffStatusOK {
			t.Errorf("expected DiffStatus ok, got %q for %s", f.DiffStatus, f.Path)
		}
	}
}
