package git

import (
	"context"
	"fmt"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

const (
	// READ ONLY PERFORMANCE FIXTURE.
	// This public repository is used only for clone/fetch/diff reads. Benchmarks
	// must never create PRs, comments, review replies, approvals, or any other
	// mutation against it.
	benchT3CodeRepoURL = "https://github.com/pingdotgg/t3code.git"

	// Main-branch range fixture from the original investigation. These SHAs are
	// used only by the broad clone benchmarks so they continue measuring the
	// original "clone repo, then diff two known commits" workload.
	benchT3CodeRangeBaseSHA = "a3422a9bb51d73724b9b665ae0ef1fb756f753d1"
	benchT3CodeRangeHeadSHA = "c2776c233691ad1b6b9b891a500d9eac6c6586a2"

	// PR-specific fixture for targeted fetch benchmarks. These SHAs are the live
	// base/head of pingdotgg/t3code#3548 at the time the benchmark was authored.
	// Targeted benchmarks use this fixture so refs/pull/<N>/head resolves to the
	// pinned head SHA instead of accidentally measuring only a base-branch fetch.
	benchT3CodePRNumber   = 3548
	benchT3CodePRBaseSHA  = "31dfe3596e429725d95d31ecff396caa02a47d12"
	benchT3CodePRHeadSHA  = "cfbdaf4b4a091baf98a6984f574cf42fb401f7df"
	benchT3CodeBaseBranch = "main"
	benchT3CodeDepth      = 500
)

// benchT3CodeOpts returns the CloneOptions used to exercise the targeted /
// in-memory PR-specific paths against the t3code fixture.
func benchT3CodeOpts() *CloneOptions {
	return &CloneOptions{
		PRNumber:   benchT3CodePRNumber,
		BaseBranch: benchT3CodeBaseBranch,
		Depth:      benchT3CodeDepth,
	}
}

func BenchmarkCloneCacheHitT3Code(b *testing.B) {
	cache := NewCache()
	client := NewClient(cache)
	localPath := b.TempDir()

	cache.Set(benchT3CodeRepoURL, benchT3CodeRangeBaseSHA, benchT3CodeRangeHeadSHA, &RepoEntry{
		URL:       benchT3CodeRepoURL,
		LocalPath: localPath,
		BaseSHA:   benchT3CodeRangeBaseSHA,
		HeadSHA:   benchT3CodeRangeHeadSHA,
		FetchedAt: time.Now(),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := client.Clone(context.Background(), benchT3CodeRepoURL, benchT3CodeRangeBaseSHA, benchT3CodeRangeHeadSHA)
		if err != nil {
			b.Fatalf("Clone cache hit: %v", err)
		}
		if result.LocalPath != localPath {
			b.Fatalf("expected cached path %s, got %s", localPath, result.LocalPath)
		}
	}
}

func BenchmarkCloneColdMissT3Code(b *testing.B) {
	// Keep benchmark clones isolated from the user's long-lived Bistro cache while
	// still exercising the same Clone implementation and APFS writes.
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		result, err := client.Clone(context.Background(), benchT3CodeRepoURL, benchT3CodeRangeBaseSHA, benchT3CodeRangeHeadSHA)
		if err != nil {
			b.Fatalf("Clone cold miss: %v", err)
		}
		if result.LocalPath == "" {
			b.Fatal("expected clone local path")
		}
	}
}

func BenchmarkGenerateDiffT3CodeWarmClone(b *testing.B) {
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())
	client := NewClient(NewCache())
	cloneResult, err := client.Clone(context.Background(), benchT3CodeRepoURL, benchT3CodeRangeBaseSHA, benchT3CodeRangeHeadSHA)
	if err != nil {
		b.Fatalf("setup clone: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := client.GenerateDiff(cloneResult.LocalPath, benchT3CodeRangeBaseSHA, benchT3CodeRangeHeadSHA)
		if err != nil {
			b.Fatalf("GenerateDiff: %v", err)
		}
		if len(result.Files) == 0 {
			b.Fatal("expected non-empty diff")
		}
	}
}

func BenchmarkCloneAndGenerateDiffT3Code(b *testing.B) {
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		cloneResult, err := client.Clone(context.Background(), benchT3CodeRepoURL, benchT3CodeRangeBaseSHA, benchT3CodeRangeHeadSHA)
		if err != nil {
			b.Fatalf("Clone: %v", err)
		}
		result, err := client.GenerateDiff(cloneResult.LocalPath, benchT3CodeRangeBaseSHA, benchT3CodeRangeHeadSHA)
		if err != nil {
			b.Fatalf("GenerateDiff: %v", err)
		}
		if len(result.Files) == 0 {
			b.Fatal("expected non-empty diff")
		}
	}
}

// BenchmarkCloneForPRTargetedColdMissT3Code measures the disk-backed TARGETED
// clone (CloneForPR): bare init + PR-specific shallow fetch into a fresh temp
// dir, with no warm cache. Compare against BenchmarkCloneColdMissT3Code (broad
// Clone) to see the targeted-fetch improvement.
func BenchmarkCloneForPRTargetedColdMissT3Code(b *testing.B) {
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		result, err := client.CloneForPR(context.Background(), benchT3CodeRepoURL,
			benchT3CodePRBaseSHA, benchT3CodePRHeadSHA, benchT3CodeOpts())
		if err != nil {
			b.Fatalf("CloneForPR targeted: %v", err)
		}
		assertBenchmarkPRRefMatchesHead(b, result.LocalPath)
		if result.LocalPath == "" {
			b.Fatal("expected clone local path")
		}
	}
}

// BenchmarkCloneForPRTargetedAndGenerateDiffT3Code measures the full disk
// targeted path end to end: targeted shallow clone + on-disk GenerateDiff.
func BenchmarkCloneForPRTargetedAndGenerateDiffT3Code(b *testing.B) {
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		result, err := client.CloneForPR(context.Background(), benchT3CodeRepoURL,
			benchT3CodePRBaseSHA, benchT3CodePRHeadSHA, benchT3CodeOpts())
		if err != nil {
			b.Fatalf("CloneForPR targeted: %v", err)
		}
		assertBenchmarkPRRefMatchesHead(b, result.LocalPath)
		diff, err := client.GenerateDiff(result.LocalPath, benchT3CodePRBaseSHA, benchT3CodePRHeadSHA)
		if err != nil {
			b.Fatalf("GenerateDiff: %v", err)
		}
		if len(diff.Files) == 0 {
			b.Fatal("expected non-empty diff")
		}
	}
}

// BenchmarkGenerateDiffForPRInMemoryT3Code measures the Phase 4 in-memory
// prototype: PR-specific shallow fetch into a memory.Storage + diff straight
// from the in-memory object store, with no disk writes. Compare against
// BenchmarkCloneForPRTargetedAndGenerateDiffT3Code (disk targeted + diff).
func BenchmarkGenerateDiffForPRInMemoryT3Code(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		diff, err := client.GenerateDiffForPRInMemory(context.Background(), benchT3CodeRepoURL,
			benchT3CodePRBaseSHA, benchT3CodePRHeadSHA, benchT3CodeOpts())
		if err != nil {
			b.Fatalf("GenerateDiffForPRInMemory: %v", err)
		}
		if len(diff.Files) == 0 {
			b.Fatal("expected non-empty diff")
		}
	}
}

func assertBenchmarkPRRefMatchesHead(b *testing.B, localPath string) {
	b.Helper()
	repo, err := gogit.PlainOpen(localPath)
	if err != nil {
		b.Fatalf("open targeted benchmark repo: %v", err)
	}
	refName := plumbing.ReferenceName(fmt.Sprintf("refs/bistro/pull/%d/head", benchT3CodePRNumber))
	ref, err := repo.Reference(refName, true)
	if err != nil {
		b.Fatalf("targeted benchmark did not fetch PR ref %s: %v", refName, err)
	}
	if got, want := ref.Hash().String(), benchT3CodePRHeadSHA; got != want {
		b.Fatalf("targeted benchmark PR ref mismatch: got %s, want %s", got, want)
	}
}
