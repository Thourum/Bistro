package github

import (
	"testing"
	"time"

	ghlib "github.com/google/go-github/v84/github"
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
