package prsync

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
)

func TestFetchPRDetails_PopulatesThreads(t *testing.T) {
	m := mock.NewMockProvider()
	m.SetPRs([]*provider.PR{{
		ID: "github:o/r:1", Provider: "github", Owner: "o", Repo: "r", Number: 1,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "h",
	}})
	m.SetReviewThreads([]*provider.ReviewThread{
		{ID: "t1", PRID: "github:o/r:1", StateKnown: true},
	})

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	orch := NewOrchestrator(log, m, nil, nil)
	results, err := orch.FetchPRDetails(context.Background(),
		m.PRs(), true, false, false)
	if err != nil {
		t.Fatalf("FetchPRDetails: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results want 1", len(results))
	}
	if len(results[0].Threads) != 1 || results[0].Threads[0].ID != "t1" {
		t.Errorf("Threads: got %+v", results[0].Threads)
	}
	if !results[0].CommentsFetched || !results[0].ThreadsFetched {
		t.Errorf("fetched flags = comments:%t threads:%t, want both true", results[0].CommentsFetched, results[0].ThreadsFetched)
	}
}

func TestFetchPRDetails_NoThreadProvider_SkipsGracefully(t *testing.T) {
	type noThreads struct{ provider.Provider }
	m := mock.NewMockProvider()
	m.SetPRs([]*provider.PR{{
		ID: "github:o/r:2", Provider: "github", Owner: "o", Repo: "r", Number: 2,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "h2",
	}})
	wrapped := noThreads{m}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	orch := NewOrchestrator(log, wrapped, nil, nil)
	results, err := orch.FetchPRDetails(context.Background(),
		m.PRs(), true, false, false)
	if err != nil {
		t.Fatalf("FetchPRDetails: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if len(results[0].Threads) != 0 {
		t.Errorf("no-ThreadProvider: expected 0 threads, got %d", len(results[0].Threads))
	}
	if !results[0].CommentsFetched || !results[0].ThreadsFetched {
		t.Errorf("fetched flags = comments:%t threads:%t, want both true", results[0].CommentsFetched, results[0].ThreadsFetched)
	}
}

// TestFetchPRDetails_ThreadProviderError_PRDataStillPersisted (finding 1):
// A failing GetReviewThreads must NOT set result.Error — the PR data must
// still be present and writable.
func TestFetchPRDetails_ThreadProviderError_PRDataStillPersisted(t *testing.T) {
	m := mock.NewMockProvider()
	pr := &provider.PR{
		ID: "github:o/r:3", Provider: "github", Owner: "o", Repo: "r", Number: 3,
		Title: "T", Author: "a", State: "open",
		BaseBranch: "main", HeadBranch: "f", BaseSHA: "b", HeadSHA: "h3",
	}
	m.SetPRs([]*provider.PR{pr})
	m.SetReviewThreadsError(errors.New("graphql unavailable"))

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	orch := NewOrchestrator(log, m, nil, nil)
	results, err := orch.FetchPRDetails(context.Background(),
		[]*provider.PR{pr}, true, false, false)
	if err != nil {
		t.Fatalf("FetchPRDetails returned fatal error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results want 1", len(results))
	}
	r := results[0]
	// result.Error must be nil — thread failure must NOT propagate here.
	if r.Error != nil {
		t.Errorf("result.Error should be nil when only threads fail, got: %v", r.Error)
	}
	if r.PR == nil || r.PR.ID != pr.ID {
		t.Errorf("PR data missing from result")
	}
	if len(r.Threads) != 0 {
		t.Errorf("expected 0 threads on error, got %d", len(r.Threads))
	}
	if r.ThreadsFetched {
		t.Error("ThreadsFetched must be false when the thread refresh fails")
	}
}
