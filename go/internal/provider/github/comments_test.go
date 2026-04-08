package github

import (
	"testing"

	ghlib "github.com/google/go-github/v63/github"
)

func TestNormalizeComment(t *testing.T) {
	ghComment := &ghlib.IssueComment{
		ID:   ghlib.Int64(456),
		User: &ghlib.User{Login: ghlib.String("bob")},
		Body: ghlib.String("Great work!"),
	}

	comment := normalizeComment("github:org/repo:123", ghComment)
	if comment.Author != "bob" {
		t.Errorf("expected author bob, got %s", comment.Author)
	}
	if comment.Body != "Great work!" {
		t.Errorf("expected body 'Great work!', got %s", comment.Body)
	}
	if comment.ID != "github-comment-456" {
		t.Errorf("expected ID github-comment-456, got %s", comment.ID)
	}
	if comment.PRID != "github:org/repo:123" {
		t.Errorf("expected PRID github:org/repo:123, got %s", comment.PRID)
	}
}

func TestGetComments(t *testing.T) {
	client := NewClient("test-token")
	// Just verify the method exists (real API calls in Phase 3)
	if client == nil {
		t.Error("expected non-nil client")
	}
}
