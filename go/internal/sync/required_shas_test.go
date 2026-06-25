package sync

import (
	"context"
	"testing"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
)

func intPtr(n int) *int { return &n }

// TestHasAnchorableOriginalPosition covers the routing predicate used to decide
// whether patchset BaseSHAs must be folded into RequiredSHAs.
func TestHasAnchorableOriginalPosition(t *testing.T) {
	pos := 5
	orig := 4

	cases := []struct {
		name    string
		comment *provider.Comment
		want    bool
	}{
		{
			name:    "nil comment",
			comment: nil,
			want:    false,
		},
		{
			name:    "issue kind ignored",
			comment: &provider.Comment{Kind: "issue", FilePath: "f", OriginalPosition: &orig},
			want:    false,
		},
		{
			name:    "no file path",
			comment: &provider.Comment{Kind: "review", OriginalPosition: &orig},
			want:    false,
		},
		{
			name:    "current-position only (not original)",
			comment: &provider.Comment{Kind: "review", FilePath: "f", CommitSHA: "x", Position: &pos},
			want:    false,
		},
		{
			name:    "anchorable original-position",
			comment: &provider.Comment{Kind: "review", FilePath: "f", OriginalPosition: &orig},
			want:    true,
		},
		{
			name:    "not anchorable (no positions)",
			comment: &provider.Comment{Kind: "review", FilePath: "f"},
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hasAnchorableOriginalPosition([]*provider.Comment{tc.comment}, nil)
			if got != tc.want {
				t.Errorf("comments path: got %v, want %v", got, tc.want)
			}
			// Same comment via a thread should yield the same result.
			th := []*provider.ReviewThread{{Comments: []*provider.Comment{tc.comment}}}
			if got := hasAnchorableOriginalPosition(nil, th); got != tc.want {
				t.Errorf("threads path: got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCollectPatchsetBaseSHAs_IncludesDedup verifies patchset BaseSHAs are
// collected (deduped against existing) when an anchorable original-position
// comment is present and the store has patchsets.
func TestCollectPatchsetBaseSHAs_IncludesDedup(t *testing.T) {
	db := newSyncTestDB(t)
	mustInsertPRForAnchorTest(t, db, "pr-ps-1")
	if _, err := db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-ps-1', 1, 'h1', 'base-1', CURRENT_TIMESTAMP),
		       ('pr-ps-1', 2, 'h2', 'base-2', CURRENT_TIMESTAMP),
		       ('pr-ps-1', 3, 'h3', '',       CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert patchsets: %v", err)
	}

	o := &Orchestrator{
		anchorResolver: git.NewAnchorResolver(silentLog()),
		store:          db,
	}
	orig := 4
	comments := []*provider.Comment{{
		ID: "c1", Kind: "review", FilePath: "foo.go", OriginalPosition: &orig,
	}}

	// existing already contains base-1, so only base-2 should be added (empty
	// base skipped, base-1 deduped).
	got := o.collectPatchsetBaseSHAs(context.Background(), "pr-ps-1", comments, nil, []string{"base-1"})
	if len(got) != 1 || got[0] != "base-2" {
		t.Fatalf("expected [base-2], got %v", got)
	}
}

// TestCollectPatchsetBaseSHAs_NoOriginalPosition returns nil when there is no
// anchorable original-position comment (no need to widen the fetch).
func TestCollectPatchsetBaseSHAs_NoOriginalPosition(t *testing.T) {
	db := newSyncTestDB(t)
	mustInsertPRForAnchorTest(t, db, "pr-ps-2")
	if _, err := db.Exec(`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at)
		VALUES ('pr-ps-2', 1, 'h1', 'base-1', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatalf("insert patchset: %v", err)
	}
	o := &Orchestrator{
		anchorResolver: git.NewAnchorResolver(silentLog()),
		store:          db,
	}
	// Current-position comment only — routes through prBaseSHA, not patchsets.
	comments := []*provider.Comment{{
		ID: "c1", Kind: "review", FilePath: "foo.go", CommitSHA: "x", Position: intPtr(3),
	}}
	if got := o.collectPatchsetBaseSHAs(context.Background(), "pr-ps-2", comments, nil, nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// TestCollectPatchsetBaseSHAs_NoResolverOrStore returns nil when anchoring is
// disabled or there is no store, regardless of comments.
func TestCollectPatchsetBaseSHAs_NoResolverOrStore(t *testing.T) {
	orig := 4
	comments := []*provider.Comment{{
		ID: "c1", Kind: "review", FilePath: "foo.go", OriginalPosition: &orig,
	}}

	// No anchorResolver.
	o1 := &Orchestrator{store: newSyncTestDB(t)}
	if got := o1.collectPatchsetBaseSHAs(context.Background(), "pr", comments, nil, nil); got != nil {
		t.Errorf("no resolver: expected nil, got %v", got)
	}

	// No store.
	o2 := &Orchestrator{anchorResolver: git.NewAnchorResolver(silentLog())}
	if got := o2.collectPatchsetBaseSHAs(context.Background(), "pr", comments, nil, nil); got != nil {
		t.Errorf("no store: expected nil, got %v", got)
	}

	// Empty prID.
	o3 := &Orchestrator{anchorResolver: git.NewAnchorResolver(silentLog()), store: newSyncTestDB(t)}
	if got := o3.collectPatchsetBaseSHAs(context.Background(), "", comments, nil, nil); got != nil {
		t.Errorf("empty prID: expected nil, got %v", got)
	}
}
