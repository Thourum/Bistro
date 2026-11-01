package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
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

func TestSubmitReview_SendsOneReviewWithLineAndRangeComments(t *testing.T) {
	type sentComment struct {
		Path      string `json:"path"`
		Body      string `json:"body"`
		Side      string `json:"side"`
		Line      int    `json:"line"`
		StartSide string `json:"start_side"`
		StartLine int    `json:"start_line"`
		Position  *int   `json:"position"`
	}
	var sent struct {
		CommitID string        `json:"commit_id"`
		Body     string        `json:"body"`
		Event    string        `json:"event"`
		Comments []sentComment `json:"comments"`
	}
	var calls int
	var gotPath string
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotPath = r.URL.Path
		gotMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1}`)) //nolint:errcheck
	}))
	defer srv.Close()

	client := NewClient("test-token")
	ghClient := ghlib.NewClient(nil)
	ghClient.BaseURL, _ = ghClient.BaseURL.Parse(srv.URL + "/")
	client.gh = ghClient

	err := client.SubmitReview(context.Background(), "github:owner/repo:42", provider.ReviewSubmission{
		Event: "REQUEST_CHANGES", Body: "Needs work", CommitID: "abc123",
		Comments: []provider.ReviewComment{
			{Path: "a.go", Side: "RIGHT", Line: 10, Body: "single"},
			{Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "range"},
		},
	})
	if err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if calls != 1 || gotMethod != http.MethodPost || gotPath != "/repos/owner/repo/pulls/42/reviews" {
		t.Fatalf("calls=%d method=%s path=%s, want one POST to the reviews endpoint", calls, gotMethod, gotPath)
	}
	if sent.Event != "REQUEST_CHANGES" || sent.Body != "Needs work" || sent.CommitID != "abc123" {
		t.Fatalf("review = %+v", sent)
	}
	want := []sentComment{
		{Path: "a.go", Body: "single", Side: "RIGHT", Line: 10},
		{Path: "b.go", Body: "range", Side: "LEFT", Line: 5, StartSide: "LEFT", StartLine: 3},
	}
	if len(sent.Comments) != len(want) {
		t.Fatalf("comments = %+v", sent.Comments)
	}
	for i, got := range sent.Comments {
		if got.Position != nil {
			t.Errorf("comment %d sent legacy position", i)
		}
		got.Position = nil
		if got != want[i] {
			t.Errorf("comment %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestSubmitReview_OptionalFieldsAndGitHubErrors(t *testing.T) {
	cases := []struct {
		name   string
		event  string
		status int
	}{
		{name: "approve without optional fields", event: "APPROVE", status: http.StatusCreated},
		{name: "comment without optional fields", event: "COMMENT", status: http.StatusCreated},
		{name: "GitHub rejects review", event: "APPROVE", status: http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sent map[string]json.RawMessage
			var calls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
					t.Errorf("decode request: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				w.Write([]byte(`{"id":1,"message":"Validation Failed"}`)) //nolint:errcheck
			}))
			defer srv.Close()
			client := NewClient("test-token")
			client.gh = ghlib.NewClient(nil)
			client.gh.BaseURL, _ = client.gh.BaseURL.Parse(srv.URL + "/")

			err := client.SubmitReview(context.Background(), "github:owner/repo:42", provider.ReviewSubmission{Event: tc.event})
			if tc.status == http.StatusUnprocessableEntity {
				if err == nil || !strings.Contains(err.Error(), "failed to submit review on PR owner/repo#42") || !strings.Contains(err.Error(), "422") {
					t.Fatalf("err = %v, want contextual GitHub rejection", err)
				}
			} else if err != nil {
				t.Fatalf("SubmitReview: %v", err)
			}
			if calls != 1 {
				t.Errorf("calls = %d, want 1", calls)
			}
			if len(sent) != 1 || string(sent["event"]) != `"`+tc.event+`"` {
				t.Errorf("request = %s, want only the event", sent)
			}
		})
	}
}

func TestSubmitReview_RejectsUnknownEventWithoutCallingGitHub(t *testing.T) {
	client := NewClient("test-token")
	ghClient := ghlib.NewClient(nil)
	ghClient.BaseURL, _ = ghClient.BaseURL.Parse("http://127.0.0.1:1/")
	client.gh = ghClient
	err := client.SubmitReview(context.Background(), "github:owner/repo:42", provider.ReviewSubmission{Event: "MERGE"})
	if err == nil || !strings.Contains(err.Error(), "invalid review event") {
		t.Fatalf("err = %v, want invalid review event", err)
	}
}
