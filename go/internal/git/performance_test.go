package git

import (
	"context"
	"fmt"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
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

	// PR-specific fixture for targeted fetch benchmarks. The PR head can move, so
	// targeted benchmarks resolve refs/heads/main and refs/pull/3548/head at run
	// time and assert the fetched private PR ref matches that live head. This
	// keeps the benchmark measuring the PR-ref path without relying on stale SHAs.
	benchT3CodePRNumber   = 3548
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

type benchT3CodePRFixture struct {
	BaseSHA string
	HeadSHA string
}

func benchT3CodeLivePRFixture(b *testing.B) benchT3CodePRFixture {
	b.Helper()
	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{benchT3CodeRepoURL},
	})
	refs, err := remote.List(&gogit.ListOptions{})
	if err != nil {
		b.Fatalf("list benchmark refs: %v", err)
	}
	baseRef := plumbing.ReferenceName("refs/heads/" + benchT3CodeBaseBranch)
	prRef := plumbing.ReferenceName(fmt.Sprintf("refs/pull/%d/head", benchT3CodePRNumber))
	fixture := benchT3CodePRFixture{}
	for _, ref := range refs {
		switch ref.Name() {
		case baseRef:
			fixture.BaseSHA = ref.Hash().String()
		case prRef:
			fixture.HeadSHA = ref.Hash().String()
		}
	}
	if fixture.BaseSHA == "" {
		b.Fatalf("benchmark base ref %s not found", baseRef)
	}
	if fixture.HeadSHA == "" {
		b.Fatalf("benchmark PR ref %s not found", prRef)
	}
	return fixture
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
	fixture := benchT3CodeLivePRFixture(b)
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		result, err := client.CloneForPR(context.Background(), benchT3CodeRepoURL,
			fixture.BaseSHA, fixture.HeadSHA, benchT3CodeOpts())
		if err != nil {
			b.Fatalf("CloneForPR targeted: %v", err)
		}
		assertBenchmarkPRRefMatchesHead(b, result.LocalPath, fixture.HeadSHA)
		if result.LocalPath == "" {
			b.Fatal("expected clone local path")
		}
	}
}

// BenchmarkCloneForPRTargetedAndGenerateDiffT3Code measures the full disk
// targeted path end to end: targeted shallow clone + on-disk GenerateDiff.
func BenchmarkCloneForPRTargetedAndGenerateDiffT3Code(b *testing.B) {
	fixture := benchT3CodeLivePRFixture(b)
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		result, err := client.CloneForPR(context.Background(), benchT3CodeRepoURL,
			fixture.BaseSHA, fixture.HeadSHA, benchT3CodeOpts())
		if err != nil {
			b.Fatalf("CloneForPR targeted: %v", err)
		}
		assertBenchmarkPRRefMatchesHead(b, result.LocalPath, fixture.HeadSHA)
		diff, err := client.GenerateDiff(result.LocalPath, fixture.BaseSHA, fixture.HeadSHA)
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
	fixture := benchT3CodeLivePRFixture(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		diff, err := client.GenerateDiffForPRInMemory(context.Background(), benchT3CodeRepoURL,
			fixture.BaseSHA, fixture.HeadSHA, benchT3CodeOpts())
		if err != nil {
			b.Fatalf("GenerateDiffForPRInMemory: %v", err)
		}
		if len(diff.Files) == 0 {
			b.Fatal("expected non-empty diff")
		}
	}
}

// benchMemoryDepths is the depth matrix used by the in-memory depth-sensitivity
// benchmarks. Low depths exercise the shallow window where the PR base commit
// may not yet be reachable.
var benchMemoryDepths = []int{1, 10, 50, 500}

// BenchmarkFetchForPRInMemoryT3Code measures ONLY the in-memory targeted fetch
// (Init + CreateRemote + FetchContext + required-commit verification), with no
// diff generation. Isolating fetch from diff shows how much of the in-memory
// path cost is transport vs. tree diffing. Compare against
// BenchmarkCloneForPRTargetedColdMissT3Code (disk targeted fetch-only).
func BenchmarkFetchForPRInMemoryT3Code(b *testing.B) {
	fixture := benchT3CodeLivePRFixture(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client := NewClient(NewCache())
		repo, err := client.fetchForPRInMemory(context.Background(), benchT3CodeRepoURL,
			fixture.BaseSHA, fixture.HeadSHA, benchT3CodeOpts())
		if err != nil {
			b.Fatalf("fetchForPRInMemory: %v", err)
		}
		if repo == nil {
			b.Fatal("expected non-nil repo")
		}
	}
}

// BenchmarkFetchForPRInMemoryDepthT3Code measures the RAW in-memory fetch
// (Init + CreateRemote + FetchContext, no commit verification, no diff) across a
// depth matrix to expose depth sensitivity of the object transfer. Raw fetch is
// used because low depths legitimately may not reach the PR base commit; here we
// only want the transport cost, not a correctness gate.
func BenchmarkFetchForPRInMemoryDepthT3Code(b *testing.B) {
	// Resolve the fixture once so the benchmark fails fast if the PR ref is gone.
	_ = benchT3CodeLivePRFixture(b)
	for _, depth := range benchMemoryDepths {
		depth := depth
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			opts := &CloneOptions{
				PRNumber:   benchT3CodePRNumber,
				BaseBranch: benchT3CodeBaseBranch,
				Depth:      depth,
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				client := NewClient(NewCache())
				repo, err := client.fetchForPRInMemoryRaw(context.Background(), benchT3CodeRepoURL, opts)
				if err != nil {
					b.Fatalf("fetchForPRInMemoryRaw depth=%d: %v", depth, err)
				}
				if repo == nil {
					b.Fatal("expected non-nil repo")
				}
			}
		})
	}
}

// BenchmarkGenerateDiffForPRInMemoryDepthT3Code measures the full in-memory
// fetch+verify+diff across the depth matrix. At depths too shallow to reach the
// PR base commit the verification fails; those sub-benchmarks are skipped (not
// failed) so the matrix still reports the depths where a complete diff is
// possible and how depth affects fetch+diff cost.
func BenchmarkGenerateDiffForPRInMemoryDepthT3Code(b *testing.B) {
	fixture := benchT3CodeLivePRFixture(b)
	for _, depth := range benchMemoryDepths {
		depth := depth
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			opts := &CloneOptions{
				PRNumber:   benchT3CodePRNumber,
				BaseBranch: benchT3CodeBaseBranch,
				Depth:      depth,
			}
			// Probe once: if this depth cannot reach base, skip the sub-benchmark.
			probe := NewClient(NewCache())
			if _, err := probe.GenerateDiffForPRInMemory(context.Background(), benchT3CodeRepoURL,
				fixture.BaseSHA, fixture.HeadSHA, opts); err != nil {
				b.Skipf("depth=%d cannot complete diff (base not in shallow window): %v", depth, err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				client := NewClient(NewCache())
				diff, err := client.GenerateDiffForPRInMemory(context.Background(), benchT3CodeRepoURL,
					fixture.BaseSHA, fixture.HeadSHA, opts)
				if err != nil {
					b.Fatalf("GenerateDiffForPRInMemory depth=%d: %v", depth, err)
				}
				if len(diff.Files) == 0 {
					b.Fatal("expected non-empty diff")
				}
			}
		})
	}
}

func assertBenchmarkPRRefMatchesHead(b *testing.B, localPath, wantHeadSHA string) {
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
	if got, want := ref.Hash().String(), wantHeadSHA; got != want {
		b.Fatalf("targeted benchmark PR ref mismatch: got %s, want %s", got, want)
	}
}
