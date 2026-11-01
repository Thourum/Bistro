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

func TestCommentPositionPredicates(t *testing.T) {
	for _, test := range []struct {
		name     string
		position *int
		want     bool
	}{
		{name: "absent"},
		{name: "zero", position: ptr(0)},
		{name: "negative", position: ptr(-1)},
		{name: "positive", position: ptr(1), want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			comment := provider.Comment{Position: test.position, OriginalPosition: test.position}
			if got := comment.HasCurrentPosition(); got != test.want {
				t.Errorf("HasCurrentPosition() = %v, want %v", got, test.want)
			}
			if got := comment.HasOriginalPosition(); got != test.want {
				t.Errorf("HasOriginalPosition() = %v, want %v", got, test.want)
			}
		})
	}
}

func ptr(i int) *int { return &i }
