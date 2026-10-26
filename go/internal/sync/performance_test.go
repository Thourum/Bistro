package sync

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/provider"
	ghprovider "github.com/itterno/bistro/internal/provider/github"
	"github.com/itterno/bistro/internal/store"
)

const (
	// READ ONLY PERFORMANCE FIXTURE.
	// This public repository is used only for fetching/listing/diff speed tests.
	// Benchmarks must never create PRs, comments, review replies, approvals,
	// request-changes reviews, or any other mutation against it.
	benchT3CodeOwner  = "pingdotgg"
	benchT3CodeRepo   = "t3code"
	benchT3CodePR     = 3548
	benchT3CodePRID   = "github:pingdotgg/t3code:3548"
	benchT3CodeGitURL = "https://github.com/pingdotgg/t3code.git"

	// Internal mutation fixture for future write-action tests only. If we add
	// mutation benchmarks, they must target only this repository and only README
	// changes/comments, never pingdotgg/t3code.
	benchInternalMutationRepoURL = "https://github.com/Thourum/testing"
)

type readOnlyBenchmarkProvider struct {
	inner provider.Provider
}

func newReadOnlyBenchmarkProvider() *readOnlyBenchmarkProvider {
	return &readOnlyBenchmarkProvider{inner: ghprovider.NewClient(os.Getenv("GITHUB_TOKEN"))}
}

func (p *readOnlyBenchmarkProvider) Authenticate(ctx context.Context) (string, error) {
	return p.inner.Authenticate(ctx)
}

func (p *readOnlyBenchmarkProvider) ListPRs(ctx context.Context, filters provider.PRFilters) ([]*provider.PR, error) {
	if filters.Owner != benchT3CodeOwner || filters.Repo != benchT3CodeRepo {
		return nil, fmt.Errorf("read-only benchmark provider only supports %s/%s", benchT3CodeOwner, benchT3CodeRepo)
	}
	pr, err := p.inner.GetPR(ctx, benchT3CodePRID)
	if err != nil {
		return nil, err
	}
	return []*provider.PR{pr}, nil
}

func (p *readOnlyBenchmarkProvider) GetPR(ctx context.Context, id string) (*provider.PR, error) {
	if id != benchT3CodePRID {
		return nil, fmt.Errorf("read-only benchmark provider only supports PR %s", benchT3CodePRID)
	}
	return p.inner.GetPR(ctx, id)
}

func (p *readOnlyBenchmarkProvider) GetComments(ctx context.Context, prID string) ([]*provider.Comment, error) {
	if prID != benchT3CodePRID {
		return nil, fmt.Errorf("read-only benchmark provider only supports PR %s", benchT3CodePRID)
	}
	return p.inner.GetComments(ctx, prID)
}

func (p *readOnlyBenchmarkProvider) ListReviews(ctx context.Context, prID string) ([]*provider.Review, error) {
	if prID != benchT3CodePRID {
		return nil, fmt.Errorf("read-only benchmark provider only supports PR %s", benchT3CodePRID)
	}
	return p.inner.ListReviews(ctx, prID)
}

func (p *readOnlyBenchmarkProvider) GetReviewThreads(ctx context.Context, prID string) ([]*provider.ReviewThread, error) {
	if prID != benchT3CodePRID {
		return nil, fmt.Errorf("read-only benchmark provider only supports PR %s", benchT3CodePRID)
	}
	threadProvider, ok := p.inner.(provider.ThreadProvider)
	if !ok {
		return nil, nil
	}
	return threadProvider.GetReviewThreads(ctx, prID)
}

func (p *readOnlyBenchmarkProvider) GetCIStatus(ctx context.Context, prID string) (*provider.CIStatus, error) {
	if prID != benchT3CodePRID {
		return nil, fmt.Errorf("read-only benchmark provider only supports PR %s", benchT3CodePRID)
	}
	return p.inner.GetCIStatus(ctx, prID)
}

func (p *readOnlyBenchmarkProvider) Approve(context.Context, string) error {
	return fmt.Errorf("read-only benchmark provider: refusing to approve; mutation tests must use %s only", benchInternalMutationRepoURL)
}

func (p *readOnlyBenchmarkProvider) RequestChanges(context.Context, string, string) error {
	return fmt.Errorf("read-only benchmark provider: refusing to request changes; mutation tests must use %s only", benchInternalMutationRepoURL)
}

func (p *readOnlyBenchmarkProvider) PostComment(context.Context, string, int, string) error {
	return fmt.Errorf("read-only benchmark provider: refusing to comment; mutation tests must use %s only", benchInternalMutationRepoURL)
}

func (p *readOnlyBenchmarkProvider) GetUsername(ctx context.Context) (string, error) {
	return p.inner.GetUsername(ctx)
}

func (p *readOnlyBenchmarkProvider) ListUserRepos(ctx context.Context) ([]*provider.Repo, error) {
	return p.inner.ListUserRepos(ctx)
}

func benchmarkLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func openBenchmarkDB(tb testing.TB) *store.DB {
	tb.Helper()
	db, err := store.OpenDB(":memory:")
	if err != nil {
		tb.Fatalf("open benchmark DB: %v", err)
	}
	if err := store.NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		db.Close()
		tb.Fatalf("migrate benchmark DB: %v", err)
	}
	return db
}

func fetchBenchmarkPR(tb testing.TB, prov *readOnlyBenchmarkProvider) *provider.PR {
	tb.Helper()
	pr, err := prov.GetPR(context.Background(), benchT3CodePRID)
	if err != nil {
		tb.Fatalf("fetch benchmark PR #%d: %v", benchT3CodePR, err)
	}
	return pr
}

func fetchBenchmarkPRDetails(tb testing.TB) []*PRFetchResult {
	tb.Helper()
	prov := newReadOnlyBenchmarkProvider()
	pr := fetchBenchmarkPR(tb, prov)
	tb.Setenv("BISTRO_REPO_DIR", tb.TempDir())
	orch := NewOrchestrator(benchmarkLogger(), prov, nil, git.NewClient(clonecache.New()))
	results, err := orch.FetchPRDetails(context.Background(), []*provider.PR{pr}, true, true, true)
	if err != nil {
		tb.Fatalf("FetchPRDetails setup: %v", err)
	}
	if len(results) != 1 || results[0] == nil || results[0].PR == nil {
		tb.Fatalf("expected one fetched PR result, got %d", len(results))
	}
	if results[0].Error != nil {
		tb.Fatalf("benchmark PR detail fetch had per-PR error: %v", results[0].Error)
	}
	if !results[0].DiffComputed || len(results[0].DiffFiles) == 0 {
		tb.Fatal("expected real t3code PR diff files")
	}
	return results
}

func BenchmarkFetchPRDetailsT3CodeXXLPRLive(b *testing.B) {
	prov := newReadOnlyBenchmarkProvider()
	pr := fetchBenchmarkPR(b, prov)
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		orch := NewOrchestrator(benchmarkLogger(), prov, nil, git.NewClient(clonecache.New()))
		results, err := orch.FetchPRDetails(context.Background(), []*provider.PR{pr}, true, true, true)
		if err != nil {
			b.Fatalf("FetchPRDetails: %v", err)
		}
		if len(results) != 1 || results[0] == nil || results[0].Error != nil {
			b.Fatalf("expected one successful PR result, got %#v", results)
		}
		if !results[0].DiffComputed || len(results[0].DiffFiles) == 0 {
			b.Fatal("expected real t3code PR diff files")
		}
	}
}

func BenchmarkWriteSyncResultsT3CodeXXLPRLive(b *testing.B) {
	results := fetchBenchmarkPRDetails(b)

	b.ResetTimer()
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		db := openBenchmarkDB(b)
		writer := NewWriter(benchmarkLogger(), db)
		b.StartTimer()
		if err := writer.WriteSyncResultsContext(context.Background(), results); err != nil {
			b.Fatalf("WriteSyncResultsContext: %v", err)
		}
		b.StopTimer()
		db.Close()
	}
}

func BenchmarkTypicalSyncT3CodeXXLPRLive(b *testing.B) {
	prov := newReadOnlyBenchmarkProvider()
	b.Setenv("BISTRO_REPO_DIR", b.TempDir())

	b.ResetTimer()
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		db := openBenchmarkDB(b)
		orch := NewOrchestrator(benchmarkLogger(), prov, db, git.NewClient(clonecache.New()))
		b.StartTimer()
		result, err := orch.Sync(context.Background(), &SyncRequest{
			Provider:        "github",
			Owner:           benchT3CodeOwner,
			Repo:            benchT3CodeRepo,
			IncludePRs:      true,
			IncludeComments: true,
			IncludeCI:       true,
			IncludeDiff:     true,
			State:           "open",
		})
		b.StopTimer()
		if err != nil {
			b.Fatalf("Sync: %v", err)
		}
		if result.PRCount != 1 {
			b.Fatalf("expected one benchmark PR, got %d", result.PRCount)
		}
		if len(result.Errors) > 0 {
			b.Fatalf("expected no sync errors, got %v", result.Errors)
		}
		db.Close()
	}
}
