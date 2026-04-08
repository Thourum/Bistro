package github

import (
	"context"
	"testing"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

// TestClientImplementsProvider verifies the compile-time interface check holds.
func TestClientImplementsProvider(t *testing.T) {
	// This is a compile-time assertion — if Client does not satisfy provider.Provider,
	// this file will not compile.
	var _ provider.Provider = (*Client)(nil)

	client := NewClient("test-token")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

// TestParsePRID_Valid covers the happy path of parsePRID.
func TestParsePRID_Valid(t *testing.T) {
	cases := []struct {
		id             string
		wantOwner      string
		wantRepo       string
		wantNumber     int
	}{
		{"github:myorg/myrepo:42", "myorg", "myrepo", 42},
		{"github:a/b:1", "a", "b", 1},
		{"github:owner/my-repo:9999", "owner", "my-repo", 9999},
	}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			owner, repo, number, err := parsePRID(tc.id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tc.wantOwner {
				t.Errorf("owner: want %q, got %q", tc.wantOwner, owner)
			}
			if repo != tc.wantRepo {
				t.Errorf("repo: want %q, got %q", tc.wantRepo, repo)
			}
			if number != tc.wantNumber {
				t.Errorf("number: want %d, got %d", tc.wantNumber, number)
			}
		})
	}
}

// TestParsePRID_Invalid covers all malformed ID cases.
func TestParsePRID_Invalid(t *testing.T) {
	cases := []struct {
		name string
		id   string
	}{
		{"wrong provider", "gitlab:org/repo:1"},
		{"missing number", "github:org/repo"},
		{"empty owner", "github:/repo:1"},
		{"empty repo", "github:owner/:1"},
		{"non-numeric number", "github:org/repo:abc"},
		{"zero number", "github:org/repo:0"},
		{"negative number", "github:org/repo:-1"},
		{"empty string", ""},
		{"no colons", "githuborgrepo1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := parsePRID(tc.id)
			if err == nil {
				t.Errorf("expected error for ID %q, got nil", tc.id)
			}
		})
	}
}

// TestAuthenticate_WithToken verifies that a pre-set token is returned directly.
func TestAuthenticate_WithToken(t *testing.T) {
	client := NewClient("existing-token")
	ctx := context.Background()

	token, err := client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "existing-token" {
		t.Errorf("expected token %q, got %q", "existing-token", token)
	}
}

// TestAuthenticate_NoToken verifies that an empty token returns an error.
func TestAuthenticate_NoToken(t *testing.T) {
	client := NewClient("")
	ctx := context.Background()

	_, err := client.Authenticate(ctx)
	if err == nil {
		t.Error("expected error when no token set, got nil")
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

// TestGetComments_InvalidID verifies GetComments returns an error for a malformed ID.
func TestGetComments_InvalidID(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.GetComments(ctx, "invalid-id")
	if err == nil {
		t.Error("expected error for invalid PR ID, got nil")
	}
}

// TestGetCIStatus_InvalidID verifies GetCIStatus returns an error for a malformed ID.
func TestGetCIStatus_InvalidID(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.GetCIStatus(ctx, "invalid-id")
	if err == nil {
		t.Error("expected error for invalid PR ID, got nil")
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

// TestListPRs_ReturnsNotImplementedError verifies ListPRs explains the limitation.
func TestListPRs_ReturnsNotImplementedError(t *testing.T) {
	client := NewClient("test-token")
	ctx := context.Background()

	_, err := client.ListPRs(ctx, provider.PRFilters{State: "open"})
	if err == nil {
		t.Error("expected error from ListPRs (requires owner/repo), got nil")
	}
}
