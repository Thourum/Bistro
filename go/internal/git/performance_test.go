package git

import (
	"context"
	"testing"
	"time"
)

const (
	// READ ONLY PERFORMANCE FIXTURE.
	// This public repository is used only for clone/fetch/diff reads. Benchmarks
	// must never create PRs, comments, review replies, approvals, or any other
	// mutation against it.
	benchT3CodeRepoURL = "https://github.com/pingdotgg/t3code.git"
	benchT3CodeBaseSHA = "a3422a9bb51d73724b9b665ae0ef1fb756f753d1"
	benchT3CodeHeadSHA = "c2776c233691ad1b6b9b891a500d9eac6c6586a2"
)

func BenchmarkCloneCacheHitT3Code(b *testing.B) {
	cache := NewCache()
	client := NewClient(cache)
	localPath := b.TempDir()

	cache.Set(benchT3CodeRepoURL, benchT3CodeBaseSHA, benchT3CodeHeadSHA, &RepoEntry{
		URL:       benchT3CodeRepoURL,
		LocalPath: localPath,
		BaseSHA:   benchT3CodeBaseSHA,
		HeadSHA:   benchT3CodeHeadSHA,
		FetchedAt: time.Now(),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := client.Clone(context.Background(), benchT3CodeRepoURL, benchT3CodeBaseSHA, benchT3CodeHeadSHA)
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
		result, err := client.Clone(context.Background(), benchT3CodeRepoURL, benchT3CodeBaseSHA, benchT3CodeHeadSHA)
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
	cloneResult, err := client.Clone(context.Background(), benchT3CodeRepoURL, benchT3CodeBaseSHA, benchT3CodeHeadSHA)
	if err != nil {
		b.Fatalf("setup clone: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := client.GenerateDiff(cloneResult.LocalPath, benchT3CodeBaseSHA, benchT3CodeHeadSHA)
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
		cloneResult, err := client.Clone(context.Background(), benchT3CodeRepoURL, benchT3CodeBaseSHA, benchT3CodeHeadSHA)
		if err != nil {
			b.Fatalf("Clone: %v", err)
		}
		result, err := client.GenerateDiff(cloneResult.LocalPath, benchT3CodeBaseSHA, benchT3CodeHeadSHA)
		if err != nil {
			b.Fatalf("GenerateDiff: %v", err)
		}
		if len(result.Files) == 0 {
			b.Fatal("expected non-empty diff")
		}
	}
}
