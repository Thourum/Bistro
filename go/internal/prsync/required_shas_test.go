package prsync

import (
	"testing"

	"github.com/itterno/bistro/internal/provider"
)

// TestDiffCloneOptions_ExcludesRequiredSHAs is the Phase 6 regression guard: the
// diff critical-path clone options must carry only PRNumber + BaseBranch and
// must NEVER include RequiredSHAs. Folding anchor/historical SHAs here could
// force a broad full clone before the diff is persisted, which Phase 6 removed.
func TestDiffCloneOptions_ExcludesRequiredSHAs(t *testing.T) {
	pr := &provider.PR{
		Number:     4242,
		BaseBranch: "main",
		BaseSHA:    "base-sha",
		HeadSHA:    "head-sha",
	}
	opts := diffCloneOptions(pr)
	if opts == nil {
		t.Fatal("expected non-nil options")
	}
	if opts.PRNumber != 4242 {
		t.Errorf("PRNumber: got %d, want 4242", opts.PRNumber)
	}
	if opts.BaseBranch != "main" {
		t.Errorf("BaseBranch: got %q, want \"main\"", opts.BaseBranch)
	}
	if len(opts.RequiredSHAs) != 0 {
		t.Errorf("diff clone must not carry RequiredSHAs, got %v", opts.RequiredSHAs)
	}
}

// TestDiffCloneOptions_NilPR keeps the helper total: a nil PR yields empty
// options rather than panicking.
func TestDiffCloneOptions_NilPR(t *testing.T) {
	opts := diffCloneOptions(nil)
	if opts == nil {
		t.Fatal("expected non-nil options for nil PR")
	}
	if opts.PRNumber != 0 || opts.BaseBranch != "" || len(opts.RequiredSHAs) != 0 {
		t.Errorf("expected zero-value options, got %+v", opts)
	}
}
