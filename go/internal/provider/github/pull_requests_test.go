package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
)

// buildTimestamp returns a fixed Timestamp for use in tests.
func buildTimestamp() ghlib.Timestamp {
	return ghlib.Timestamp{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)}
}

// buildTestGHPR returns a minimal GitHub PullRequest populated with known values.
func buildTestGHPR() *ghlib.PullRequest {
	number := 42
	title := "Test PR title"
	body := "Test PR body"
	login := "testuser"
	state := "open"
	baseRef := "main"
	baseSHA := "abc123"
	headRef := "feature/test"
	headSHA := "def456"
	isDraft := false
	ts := buildTimestamp()

	return &ghlib.PullRequest{
		Number: &number,
		Title:  &title,
		Body:   &body,
		User:   &ghlib.User{Login: &login},
		State:  &state,
		Base: &ghlib.PullRequestBranch{
			Ref: &baseRef,
			SHA: &baseSHA,
		},
		Head: &ghlib.PullRequestBranch{
			Ref: &headRef,
			SHA: &headSHA,
		},
		Draft:     &isDraft,
		CreatedAt: &ts,
		UpdatedAt: &ts,
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("test-token")
	if client == nil {
		t.Error("expected non-nil client")
	}
}

func TestNewClientStoresToken(t *testing.T) {
	client := NewClient("test-token")
	if client.token != "test-token" {
		t.Errorf("expected token %q, got %q", "test-token", client.token)
	}
}

func TestNewClientCreatesGHClient(t *testing.T) {
	client := NewClient("test-token")
	if client.gh == nil {
		t.Error("expected non-nil github client")
	}
}

func TestNormalizePR(t *testing.T) {
	pr := buildTestGHPR()

	result := normalizePR("myorg", "myrepo", pr)

	if result.ID != "github:myorg/myrepo:42" {
		t.Errorf("unexpected ID: %q", result.ID)
	}
	if result.Provider != "github" {
		t.Errorf("unexpected Provider: %q", result.Provider)
	}
	if result.Owner != "myorg" {
		t.Errorf("unexpected Owner: %q", result.Owner)
	}
	if result.Repo != "myrepo" {
		t.Errorf("unexpected Repo: %q", result.Repo)
	}
	if result.Number != 42 {
		t.Errorf("unexpected Number: %d", result.Number)
	}
	if result.Title != "Test PR title" {
		t.Errorf("unexpected Title: %q", result.Title)
	}
	if result.Description != "Test PR body" {
		t.Errorf("unexpected Description: %q", result.Description)
	}
	if result.Author != "testuser" {
		t.Errorf("unexpected Author: %q", result.Author)
	}
	if result.State != "open" {
		t.Errorf("unexpected State: %q", result.State)
	}
	if result.BaseBranch != "main" {
		t.Errorf("unexpected BaseBranch: %q", result.BaseBranch)
	}
	if result.HeadBranch != "feature/test" {
		t.Errorf("unexpected HeadBranch: %q", result.HeadBranch)
	}
	if result.BaseSHA != "abc123" {
		t.Errorf("unexpected BaseSHA: %q", result.BaseSHA)
	}
	if result.HeadSHA != "def456" {
		t.Errorf("unexpected HeadSHA: %q", result.HeadSHA)
	}
	if result.Draft != false {
		t.Error("unexpected Draft: expected false")
	}
	if result.MergedAt != nil {
		t.Error("expected nil MergedAt for open PR")
	}
}

func TestNormalizePRRequestedReviewers(t *testing.T) {
	pr := buildTestGHPR()
	alice := "alice"
	bob := "bob"
	pr.RequestedReviewers = []*ghlib.User{
		{Login: &alice},
		{Login: &bob},
		nil, // defensive: API can theoretically return nulls
	}

	result := normalizePR("myorg", "myrepo", pr)

	if len(result.RequestedReviewers) != 2 {
		t.Fatalf("got %d requested reviewers, want 2: %v",
			len(result.RequestedReviewers), result.RequestedReviewers)
	}
	if result.RequestedReviewers[0] != "alice" || result.RequestedReviewers[1] != "bob" {
		t.Errorf("requested reviewers = %v, want [alice bob]", result.RequestedReviewers)
	}
}

func TestNormalizePRNoRequestedReviewers(t *testing.T) {
	pr := buildTestGHPR()

	result := normalizePR("myorg", "myrepo", pr)

	if len(result.RequestedReviewers) != 0 {
		t.Errorf("got %v, want no requested reviewers", result.RequestedReviewers)
	}
}

func TestNormalizePRDraft(t *testing.T) {
	pr := buildTestGHPR()
	isDraft := true
	pr.Draft = &isDraft

	result := normalizePR("myorg", "myrepo", pr)

	if !result.Draft {
		t.Error("expected Draft to be true")
	}
}

func TestNormalizePRMergedAt(t *testing.T) {
	pr := buildTestGHPR()
	ts := buildTimestamp()
	pr.MergedAt = &ts

	result := normalizePR("myorg", "myrepo", pr)

	if result.MergedAt == nil {
		t.Error("expected non-nil MergedAt for merged PR")
	}
}

func TestNormalizePRIDFormat(t *testing.T) {
	cases := []struct {
		owner, repo string
		number      int
		expectedID  string
	}{
		{"org", "repo", 1, "github:org/repo:1"},
		{"myorg", "my-repo", 100, "github:myorg/my-repo:100"},
		{"a", "b", 9999, "github:a/b:9999"},
	}

	for _, tc := range cases {
		pr := buildTestGHPR()
		pr.Number = &tc.number

		result := normalizePR(tc.owner, tc.repo, pr)

		if result.ID != tc.expectedID {
			t.Errorf("owner=%q repo=%q number=%d: expected ID %q, got %q",
				tc.owner, tc.repo, tc.number, tc.expectedID, result.ID)
		}
	}
}

// TestGetPR_InvalidID verifies GetPR returns an error for a malformed ID.
func TestGetPR_InvalidID(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.GetPR(ctx, "invalid-id")
	if err == nil {
		t.Error("expected error for invalid PR ID, got nil")
	}
}

// newListPRsTestClient points a Client at a mock server returning the given
// handler for the pulls list endpoint.
func newListPRsTestClient(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	client := NewClient("test-token")
	ghClient := ghlib.NewClient(nil)
	ghClient.BaseURL, _ = ghClient.BaseURL.Parse(srv.URL + "/")
	client.gh = ghClient
	return client, srv.Close
}

func listPRsResponseJSON(entries ...string) string {
	return "[" + strings.Join(entries, ",") + "]"
}

func prJSON(number int, updatedAt string) string {
	return fmt.Sprintf(`{
		"number": %d,
		"title": "PR %d",
		"state": "open",
		"user": {"login": "alice"},
		"base": {"ref": "main", "sha": "base"},
		"head": {"ref": "feature", "sha": "head"},
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": %q
	}`, number, number, updatedAt)
}

func TestListPRsUpdatedSinceDropsStalePRsAndStopsPaginating(t *testing.T) {
	// Arrange — page 1: two recent + one stale; a page 2 request would fail
	// the test because pagination must stop inside the window cutoff.
	requests := 0
	client, closeSrv := newListPRsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests > 1 {
			t.Errorf("unexpected request %d: pagination should stop past the window", requests)
		}
		if got := r.URL.Query().Get("sort"); got != "updated" {
			t.Errorf("sort = %q, want updated", got)
		}
		if got := r.URL.Query().Get("direction"); got != "desc" {
			t.Errorf("direction = %q, want desc", got)
		}
		w.Header().Set("Link", fmt.Sprintf(`<%s?page=2>; rel="next"`, "http://"+r.Host+r.URL.Path))
		w.Write([]byte(listPRsResponseJSON(
			prJSON(3, "2026-07-10T00:00:00Z"),
			prJSON(2, "2026-07-01T00:00:00Z"),
			prJSON(1, "2020-01-01T00:00:00Z"),
		)))
	})
	defer closeSrv()

	since := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)

	// Act
	prs, err := client.ListPRs(context.Background(), provider.PRFilters{
		Owner:        "acme",
		Repo:         "api",
		UpdatedSince: &since,
		MinCount:     2,
	})

	// Assert
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d, want 2 (stale PR dropped)", len(prs))
	}
	if prs[0].Number != 3 || prs[1].Number != 2 {
		t.Fatalf("prs = %d,%d, want 3,2", prs[0].Number, prs[1].Number)
	}
}

func TestListPRsUpdatedSinceKeepsMinCountOnQuietRepo(t *testing.T) {
	// Arrange — every PR is older than the window; MinCount must still win.
	client, closeSrv := newListPRsTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(listPRsResponseJSON(
			prJSON(5, "2020-05-01T00:00:00Z"),
			prJSON(4, "2020-04-01T00:00:00Z"),
			prJSON(3, "2020-03-01T00:00:00Z"),
		)))
	})
	defer closeSrv()

	since := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)

	// Act
	prs, err := client.ListPRs(context.Background(), provider.PRFilters{
		Owner:        "acme",
		Repo:         "api",
		UpdatedSince: &since,
		MinCount:     2,
	})

	// Assert
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("len(prs) = %d, want 2 (MinCount floor)", len(prs))
	}
}
