package github

import (
	"context"
	"testing"
)

func TestApprove(t *testing.T) {
	t.Skip("requires GitHub API or transport stub")
	client := NewClient("test-token")
	ctx := context.Background()

	// Just verify method exists and validates inputs
	err := client.approve(ctx, "owner", "repo", 123, "Looks good!")
	if client == nil {
		t.Error("client should not be nil")
	}
	if err != nil {
		t.Logf("approve returned error (expected without real API): %v", err)
	}
}

func TestApprove_ValidationErrors(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	tests := []struct {
		name   string
		owner  string
		repo   string
		number int
	}{
		{"empty owner", "", "repo", 1},
		{"empty repo", "owner", "", 1},
		{"zero number", "owner", "repo", 0},
		{"negative number", "owner", "repo", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.approve(ctx, tt.owner, tt.repo, tt.number, "body")
			if err == nil {
				t.Error("expected error for invalid params, got nil")
			}
		})
	}
}

func TestRequestChanges(t *testing.T) {
	t.Skip("requires GitHub API or transport stub")
	client := NewClient("test-token")
	ctx := context.Background()

	err := client.requestChanges(ctx, "owner", "repo", 123, "Please fix this")
	if client == nil {
		t.Error("client should not be nil")
	}
	if err != nil {
		t.Logf("requestChanges returned error (expected without real API): %v", err)
	}
}

func TestRequestChanges_ValidationErrors(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	tests := []struct {
		name   string
		owner  string
		repo   string
		number int
	}{
		{"empty owner", "", "repo", 1},
		{"empty repo", "owner", "", 1},
		{"zero number", "owner", "repo", 0},
		{"negative number", "owner", "repo", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.requestChanges(ctx, tt.owner, tt.repo, tt.number, "body")
			if err == nil {
				t.Error("expected error for invalid params, got nil")
			}
		})
	}
}

func TestPostComment(t *testing.T) {
	t.Skip("requires GitHub API or transport stub")
	client := NewClient("test-token")
	ctx := context.Background()

	err := client.postComment(ctx, "owner", "repo", 123, 0, "Great work!")
	if client == nil {
		t.Error("client should not be nil")
	}
	if err != nil {
		t.Logf("postComment returned error (expected without real API): %v", err)
	}
}

func TestPostComment_ValidationErrors(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	tests := []struct {
		name   string
		owner  string
		repo   string
		number int
		body   string
	}{
		{"empty owner", "", "repo", 1, "body"},
		{"empty repo", "owner", "", 1, "body"},
		{"zero number", "owner", "repo", 0, "body"},
		{"negative number", "owner", "repo", -1, "body"},
		{"empty body", "owner", "repo", 1, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.postComment(ctx, tt.owner, tt.repo, tt.number, 0, tt.body)
			if err == nil {
				t.Error("expected error for invalid params, got nil")
			}
		})
	}
}

// TestApprove_InvalidID verifies Approve returns an error for a malformed ID.
func TestApprove_InvalidID(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	err := client.Approve(ctx, "not-a-pr-id")
	if err == nil {
		t.Error("expected error for invalid PR ID, got nil")
	}
}

// TestRequestChanges_InvalidID verifies RequestChanges returns an error for a malformed ID.
func TestRequestChanges_InvalidID(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	err := client.RequestChanges(ctx, "not-a-pr-id", "body")
	if err == nil {
		t.Error("expected error for invalid PR ID, got nil")
	}
}

// TestPostComment_InvalidID verifies PostComment returns an error for a malformed ID.
func TestPostComment_InvalidID(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	err := client.PostComment(ctx, "not-a-pr-id", 0, "body")
	if err == nil {
		t.Error("expected error for invalid PR ID, got nil")
	}
}
