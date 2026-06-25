package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	ghlib "github.com/google/go-github/v84/github"
)

func TestApprove(t *testing.T) {
	t.Skip("requires GitHub API or transport stub")
	client := NewClient("test-token")
	ctx := context.Background()

	// Just verify method exists and validates inputs
	err := client.approve(ctx, "owner", "repo", 123, "Looks good!")
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

func TestParseReviewCommentReplyID(t *testing.T) {
	cases := []struct {
		input string
		want  int64
	}{
		{"github-review-comment-123", 123},
		{"456", 456},
		{" 789 ", 789},
		{"PRRT_node", 0},
		{"github-review-comment-nope", 0},
		{"", 0},
	}
	for _, tc := range cases {
		if got := parseReviewCommentReplyID(tc.input); got != tc.want {
			t.Errorf("parseReviewCommentReplyID(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestReplyToComment_CreatesGitHubReviewReply(t *testing.T) {
	var gotPath string
	var gotMethod string
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":999,"body":"reply"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewClient("test-token")
	ghClient := ghlib.NewClient(nil)
	ghClient.BaseURL, _ = ghClient.BaseURL.Parse(srv.URL + "/")
	client.gh = ghClient

	err := client.ReplyToComment(context.Background(), "github:owner/repo:42", "github-review-comment-123", "Reply body")
	if err != nil {
		t.Fatalf("ReplyToComment failed: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/repos/owner/repo/pulls/42/comments" {
		t.Errorf("path = %s", gotPath)
	}
	if gotBody != `{"body":"Reply body","in_reply_to":123}`+"\n" {
		t.Errorf("body = %q", gotBody)
	}
}
