package provider_test

import (
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
)

func TestCommentHasAnchoringFields(t *testing.T) {
	c := provider.Comment{
		ID:               "github-review-comment-1",
		PRID:             "github:o/r:1",
		Author:           "alice",
		Body:             "nit",
		Kind:             "review",
		CommitSHA:        "abc123",
		OriginalSHA:      "def456",
		Side:             "RIGHT",
		Position:         ptr(5),
		OriginalPosition: ptr(5),
		InReplyTo:        "github-review-comment-0",
		ThreadID:         "thread-1",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if c.Kind != "review" {
		t.Errorf("Kind: got %q want 'review'", c.Kind)
	}
	if c.ThreadID != "thread-1" {
		t.Errorf("ThreadID: got %q", c.ThreadID)
	}
}

func TestReviewThreadType(t *testing.T) {
	rt := provider.ReviewThread{
		ID:         "rt-1",
		PRID:       "github:o/r:1",
		FilePath:   "main.go",
		Line:       42,
		Resolved:   false,
		Outdated:   false,
		StateKnown: true,
		Comments:   []*provider.Comment{{ID: "c1"}},
	}
	if len(rt.Comments) != 1 {
		t.Errorf("Comments: got %d want 1", len(rt.Comments))
	}
}

func ptr(i int) *int { return &i }
