package github

import (
	"context"
	"testing"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

// TestListPRsWithOwnerRepo verifies that ListPRs returns an error when
// owner or repo is missing from filters (no live API call needed).
func TestListPRsWithOwnerRepo_MissingOwner(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.ListPRs(ctx, provider.PRFilters{Owner: "", Repo: "myrepo", State: "open"})
	if err == nil {
		t.Error("expected error when owner is empty, got nil")
	}
}

func TestListPRsWithOwnerRepo_MissingRepo(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.ListPRs(ctx, provider.PRFilters{Owner: "myorg", Repo: "", State: "open"})
	if err == nil {
		t.Error("expected error when repo is empty, got nil")
	}
}

func TestListPRsWithOwnerRepo_BothMissing(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.ListPRs(ctx, provider.PRFilters{})
	if err == nil {
		t.Error("expected error when both owner and repo are empty, got nil")
	}
}

// TestListPRs_ReturnsNotImplementedError verifies ListPRs explains the limitation.
func TestListPRs_ReturnsNotImplementedError(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.ListPRs(ctx, provider.PRFilters{State: "open"})
	if err == nil {
		t.Error("expected error from ListPRs (requires owner/repo), got nil")
	}
}
