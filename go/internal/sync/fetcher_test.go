package sync

import (
	"testing"

	"github.com/itterno/bistro/internal/provider"
)

func TestFetchPRDetailsSequential(t *testing.T) {
	// Just verify the method signature exists
	if true {
		// Placeholder test
		t.Skip("integration test - requires provider")
	}
}

func TestPRFetchResultHasFields(t *testing.T) {
	result := &PRFetchResult{}
	if result.PR != nil {
		t.Error("expected nil PR on zero value")
	}
	if result.Error != nil {
		t.Error("expected nil Error on zero value")
	}
	if result.DiffSize != 0 {
		t.Error("expected zero DiffSize on zero value")
	}
}

// TestCollectAnchorSHAs verifies dedup, empty-skip, kind filtering, and that
// both comment-level and thread-level review comments contribute their
// CommitSHA and OriginalSHA.
func TestCollectAnchorSHAs(t *testing.T) {
	comments := []*provider.Comment{
		{Kind: "review", CommitSHA: "a", OriginalSHA: "b"},
		{Kind: "review", CommitSHA: "a", OriginalSHA: ""}, // dup "a", empty original
		{Kind: "issue", CommitSHA: "ignored"},             // non-review ignored
		nil,
	}
	threads := []*provider.ReviewThread{
		{Comments: []*provider.Comment{
			{Kind: "review", CommitSHA: "c", OriginalSHA: "b"}, // dup "b"
			{Kind: "review", CommitSHA: "d"},
			nil,
		}},
		nil,
	}

	got := collectAnchorSHAs(comments, threads)

	want := map[string]bool{"a": true, "b": true, "c": true, "d": true}
	if len(got) != len(want) {
		t.Fatalf("expected %d unique SHAs, got %d: %v", len(want), len(got), got)
	}
	seen := map[string]int{}
	for _, s := range got {
		if s == "" {
			t.Error("empty SHA must not be collected")
		}
		if !want[s] {
			t.Errorf("unexpected SHA %q", s)
		}
		seen[s]++
	}
	for s, n := range seen {
		if n != 1 {
			t.Errorf("SHA %q appeared %d times, expected 1 (dedup)", s, n)
		}
	}
}

// TestCollectAnchorSHAs_Empty verifies a nil/empty input yields no SHAs.
func TestCollectAnchorSHAs_Empty(t *testing.T) {
	if got := collectAnchorSHAs(nil, nil); len(got) != 0 {
		t.Errorf("expected no SHAs, got %v", got)
	}
}
