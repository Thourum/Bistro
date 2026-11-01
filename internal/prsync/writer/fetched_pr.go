package writer

import (
	"github.com/itterno/bistro/internal/git/anchor"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store/prdiff"
)

// FetchedPR holds the fetched data for a single PR, including any
// per-PR error encountered during concurrent fetching.
type FetchedPR struct {
	PR       *provider.PR
	Comments []*provider.Comment
	Threads  []*provider.ReviewThread // populated when provider implements ThreadProvider
	Reviews  []*provider.Review       // Submitted reviews; the writer derives the user's GitHub-review baseline.
	// CommentsFetched and ThreadsFetched distinguish a successful empty refresh
	// from a write where the corresponding source was not fetched. Mention state
	// may only be cleared once every source has been refreshed.
	CommentsFetched bool
	ThreadsFetched  bool
	CI              *provider.CIStatus
	DiffSize        int64
	// DiffFiles holds the per-file diffs to be persisted atomically with the PR
	// row and patchset in WriteSyncResultsContext.
	// nil means diff was not attempted or failed (preserve existing pr_diffs cache).
	// non-nil (including empty slice) means diff was successfully computed and
	// the writer should replace pr_diffs — even an empty slice clears stale rows.
	DiffFiles []prdiff.File
	// DiffComputed is true iff diff generation succeeded and DiffFiles is valid.
	// When false, the writer must not touch pr_diffs for this result.
	DiffComputed bool
	// MergeBaseSHA is the merge base the diff was generated against; empty when
	// the diff was not computed or the merge base stayed unknown (the diff then
	// used the base tip). Part of patchset identity.
	MergeBaseSHA string
	// AnchorUpdates carries per-comment anchor results computed from the clone.
	// Populated only when the clone was available and anchoring ran without a
	// fatal error. Each entry is written back to the comments table after the
	// per-PR savepoint releases (best-effort; never blocks PR/patchset commit).
	AnchorUpdates []anchor.Result
	// ListOnly marks a result written without a detail fetch (very active
	// repo over the hydration threshold). The PR row is stored hydrated=0 and
	// details are fetched on demand; an already-hydrated row is never
	// downgraded.
	ListOnly bool
	Error    error
}
