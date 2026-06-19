package sync

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
)

func TestFetchPRDetailsPopulatesReviewStatus(t *testing.T) {
	prs := []*provider.PR{
		{ID: "github:acme/api:1", Provider: "github", Owner: "acme", Repo: "api", Number: 1},
	}
	submitted := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	prov := mock.NewMockProvider().
		WithPRs(prs).
		WithReviews([]*provider.Review{
			{PRID: "github:acme/api:1", Author: "alice", State: "approved", SubmittedAt: submitted},
		})

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	orch := NewOrchestrator(log, prov, nil, nil)

	results, err := orch.FetchPRDetails(context.Background(), prs, true, false, false)
	if err != nil {
		t.Fatalf("FetchPRDetails failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	pr := results[0].PR
	if pr.ReviewStatus != "approved" {
		t.Errorf("ReviewStatus = %q, want approved", pr.ReviewStatus)
	}
	if pr.ApprovedBy != "alice" {
		t.Errorf("ApprovedBy = %q, want alice", pr.ApprovedBy)
	}
	if pr.ChangesRequestedBy != "" {
		t.Errorf("ChangesRequestedBy = %q, want empty", pr.ChangesRequestedBy)
	}
}

func TestFetchPRDetailsReviewErrorDoesNotFailPR(t *testing.T) {
	prs := []*provider.PR{
		{ID: "github:acme/api:1", Provider: "github", Owner: "acme", Repo: "api", Number: 1},
	}
	// WithError makes every provider call fail — the PR result must still
	// exist, carrying the error, with review fields left untouched.
	prov := mock.NewMockProvider().WithPRs(prs).WithError(errFetchFailed)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	orch := NewOrchestrator(log, prov, nil, nil)

	results, err := orch.FetchPRDetails(context.Background(), prs, true, false, false)
	if err != nil {
		t.Fatalf("FetchPRDetails failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Error == nil {
		t.Error("expected per-PR error to be recorded")
	}
	if results[0].PR.ReviewStatus != "" {
		t.Errorf("ReviewStatus = %q, want empty on fetch failure", results[0].PR.ReviewStatus)
	}
}

var errFetchFailed = &fetchErr{}

type fetchErr struct{}

func (*fetchErr) Error() string { return "fetch failed" }
