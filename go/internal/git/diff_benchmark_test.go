package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gitlib "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func BenchmarkGenerateDiffLarge(b *testing.B) {
	repoPath, baseSHA, headSHA := makeBenchmarkRepo(b, 1000, 10)
	client := NewClient(NewCache())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := client.GenerateDiff(repoPath, baseSHA, headSHA, DiffOptions{})
		if err != nil {
			b.Fatalf("GenerateDiff: %v", err)
		}
		if len(result.Files) != 1000 {
			b.Fatalf("GenerateDiff files=%d, want 1000", len(result.Files))
		}
		b.ReportMetric(float64(result.Profile.FileCount), "files/op")
		b.ReportMetric(float64(result.Profile.PatchBytes), "patch-bytes/op")
		b.ReportMetric(float64(result.Profile.BuildPatch.Microseconds()), "build-patch-us/op")
		b.ReportMetric(float64(result.Profile.RenderFilePatches.Microseconds()), "render-patches-us/op")
	}
}

func makeBenchmarkRepo(b *testing.B, files, changedLines int) (string, string, string) {
	b.Helper()

	dir := b.TempDir()
	repo, err := gitlib.PlainInit(dir, false)
	if err != nil {
		b.Fatalf("init benchmark repo: %v", err)
	}
	worktree, err := repo.Worktree()
	if err != nil {
		b.Fatalf("open benchmark worktree: %v", err)
	}

	for i := 0; i < files; i++ {
		path := filepath.Join(dir, "src", "generated", fmt.Sprintf("file_%05d.ts", i))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			b.Fatalf("mkdir benchmark file dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(benchmarkFileBody(i, changedLines, 0)), 0644); err != nil {
			b.Fatalf("write benchmark base file: %v", err)
		}
	}
	if _, err := worktree.Add("src"); err != nil {
		b.Fatalf("add benchmark base files: %v", err)
	}
	baseHash, err := worktree.Commit("base", &gitlib.CommitOptions{Author: benchmarkSignature()})
	if err != nil {
		b.Fatalf("commit benchmark base: %v", err)
	}
	baseSHA := baseHash.String()

	for i := 0; i < files; i++ {
		path := filepath.Join(dir, "src", "generated", fmt.Sprintf("file_%05d.ts", i))
		if err := os.WriteFile(path, []byte(benchmarkFileBody(i, changedLines, 1)), 0644); err != nil {
			b.Fatalf("write benchmark head file: %v", err)
		}
	}
	if _, err := worktree.Add("src"); err != nil {
		b.Fatalf("add benchmark head files: %v", err)
	}
	headHash, err := worktree.Commit("head", &gitlib.CommitOptions{Author: benchmarkSignature()})
	if err != nil {
		b.Fatalf("commit benchmark head: %v", err)
	}
	headSHA := headHash.String()

	return dir, baseSHA, headSHA
}

func benchmarkFileBody(fileIndex, changedLines, generation int) string {
	var b strings.Builder
	for i := 0; i < changedLines; i++ {
		fmt.Fprintf(&b, "export const value_%05d_%03d = %d;\n", fileIndex, i, i+generation)
	}
	return b.String()
}

func benchmarkSignature() *object.Signature {
	return &object.Signature{
		Name:  "Bistro Benchmark",
		Email: "benchmark@bistro.local",
		When:  time.Unix(1_700_000_000, 0),
	}
}
