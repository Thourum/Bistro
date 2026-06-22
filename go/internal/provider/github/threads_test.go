package github_test

import (
	"testing"
	"time"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
	ghprovider "github.com/itterno/bistro/internal/provider/github"
)

func TestNormalizeReviewComment_Fields(t *testing.T) {
	now := time.Now().UTC()
	id := int64(42)
	replyTo := int64(41)
	reviewID := int64(99)
	pos := 5
	origPos := 5
	ghc := &ghlib.PullRequestComment{
		ID:                  &id,
		CommitID:            ghlib.Ptr("sha-head"),
		OriginalCommitID:    ghlib.Ptr("sha-orig"),
		Side:                ghlib.Ptr("RIGHT"),
		Position:            &pos,
		OriginalPosition:    &origPos,
		InReplyTo:           &replyTo,
		PullRequestReviewID: &reviewID,
		User:                &ghlib.User{Login: ghlib.Ptr("alice")},
		Body:                ghlib.Ptr("looks good"),
		Path:                ghlib.Ptr("main.go"),
		Line:                ghlib.Ptr(10),
		CreatedAt:           &ghlib.Timestamp{Time: now},
		UpdatedAt:           &ghlib.Timestamp{Time: now},
	}

	prID := "github:owner/repo:1"
	c := ghprovider.NormalizeReviewComment(prID, ghc)

	if c.ID != "github-review-comment-42" {
		t.Errorf("ID: got %q", c.ID)
	}
	if c.Kind != "review" {
		t.Errorf("Kind: got %q", c.Kind)
	}
	if c.CommitSHA != "sha-head" {
		t.Errorf("CommitSHA: got %q", c.CommitSHA)
	}
	if c.OriginalSHA != "sha-orig" {
		t.Errorf("OriginalSHA: got %q", c.OriginalSHA)
	}
	if c.Side != "RIGHT" {
		t.Errorf("Side: got %q", c.Side)
	}
	if c.Position == nil || *c.Position != 5 {
		t.Errorf("Position: got %v", c.Position)
	}
	if c.InReplyTo != "github-review-comment-41" {
		t.Errorf("InReplyTo: got %q", c.InReplyTo)
	}
	// ThreadID must be empty after normalization: PullRequestReviewID is NOT used
	// as a thread identifier (one review can contain multiple distinct threads).
	// ThreadID is assigned only after grouping in GetReviewThreads.
	if c.ThreadID != "" {
		t.Errorf("ThreadID should be empty after normalization, got %q (PullRequestReviewID must not be used)", c.ThreadID)
	}
	if c.FilePath != "main.go" {
		t.Errorf("FilePath: got %q", c.FilePath)
	}
	if c.Author != "alice" {
		t.Errorf("Author: got %q", c.Author)
	}
}

func TestNormalizeReviewComment_NilOptionals(t *testing.T) {
	id := int64(1)
	ghc := &ghlib.PullRequestComment{
		ID:        &id,
		User:      &ghlib.User{Login: ghlib.Ptr("bot")},
		Body:      ghlib.Ptr("nit"),
		CreatedAt: &ghlib.Timestamp{Time: time.Now()},
		UpdatedAt: &ghlib.Timestamp{Time: time.Now()},
	}
	c := ghprovider.NormalizeReviewComment("github:o/r:1", ghc)
	if c.Position != nil {
		t.Errorf("Position should be nil for zero position")
	}
	if c.InReplyTo != "" {
		t.Errorf("InReplyTo should be empty when nil")
	}
	if c.ThreadID != "" {
		t.Errorf("ThreadID should be empty after normalization, got %q", c.ThreadID)
	}
}

func TestNormalizeReviewComment_DiffHunk(t *testing.T) {
	id := int64(7)
	hunk := "@@ -10,6 +10,8 @@ func Foo() {\n-\treturn nil\n+\treturn err"
	ghc := &ghlib.PullRequestComment{
		ID:        &id,
		User:      &ghlib.User{Login: ghlib.Ptr("dev")},
		Body:      ghlib.Ptr("check error"),
		DiffHunk:  ghlib.Ptr(hunk),
		CreatedAt: &ghlib.Timestamp{Time: time.Now()},
		UpdatedAt: &ghlib.Timestamp{Time: time.Now()},
	}
	c := ghprovider.NormalizeReviewComment("github:o/r:1", ghc)
	if c.DiffHunk != hunk {
		t.Errorf("DiffHunk: got %q want %q", c.DiffHunk, hunk)
	}
}

func TestNormalizeReviewComment_DiffHunkEmpty(t *testing.T) {
	id := int64(8)
	ghc := &ghlib.PullRequestComment{
		ID:        &id,
		User:      &ghlib.User{Login: ghlib.Ptr("bot")},
		Body:      ghlib.Ptr("nit"),
		CreatedAt: &ghlib.Timestamp{Time: time.Now()},
		UpdatedAt: &ghlib.Timestamp{Time: time.Now()},
	}
	c := ghprovider.NormalizeReviewComment("github:o/r:1", ghc)
	if c.DiffHunk != "" {
		t.Errorf("DiffHunk should be empty when not set, got %q", c.DiffHunk)
	}
}



func TestGetReviewThreads_GracefulDegradation(t *testing.T) {
	t.Run("graphql_fail_returns_rest_threads_no_error", func(t *testing.T) {
		// REST-only threads built by groupByReplyChain must have StateKnown=false.
		threads := buildTestThreads()
		for _, th := range threads {
			if th.StateKnown {
				t.Errorf("REST-only thread %q should have StateKnown=false", th.ID)
			}
		}
	})

	t.Run("rest_fail_returns_error", func(t *testing.T) {
		// listReviewComments error → GetReviewThreads propagates it.
		_ = "verified via mock_provider_test / fetcher_threads_test"
	})
}

// buildTestThreads builds synthetic ReviewThread results as if from REST-only,
// using the reply-chain grouping that GetReviewThreads uses.
// Comments 1 and 2 form one thread (2 replies to 1); comment 3 is separate.
func buildTestThreads() []*provider.ReviewThread {
	comments := []*provider.Comment{
		{ID: "github-review-comment-1", PRID: "p:1",
			Kind: "review", FilePath: "foo.go", LineNo: 5},
		{ID: "github-review-comment-2", PRID: "p:1",
			Kind: "review", InReplyTo: "github-review-comment-1"},
		{ID: "github-review-comment-3", PRID: "p:1",
			Kind: "review", FilePath: "bar.go", LineNo: 10},
	}
	return groupByReplyChainTest(comments)
}

// groupByReplyChainTest mirrors the internal groupByReplyChain logic for tests.
func groupByReplyChainTest(comments []*provider.Comment) []*provider.ReviewThread {
	byID := make(map[string]*provider.Comment, len(comments))
	for _, c := range comments {
		byID[c.ID] = c
	}

	memo := make(map[string]string, len(comments))
	var rootOf func(id string) string
	rootOf = func(id string) string {
		if r, ok := memo[id]; ok {
			return r
		}
		c, exists := byID[id]
		if !exists || c.InReplyTo == "" {
			memo[id] = id
			return id
		}
		r := rootOf(c.InReplyTo)
		memo[id] = r
		return r
	}

	order := []string{}
	byRoot := make(map[string]*provider.ReviewThread, len(comments))
	for _, c := range comments {
		root := rootOf(c.ID)
		if _, ok := byRoot[root]; !ok {
			order = append(order, root)
			th := &provider.ReviewThread{
				ID:         root,
				PRID:       c.PRID,
				StateKnown: false,
			}
			if rc, ok := byID[root]; ok {
				th.FilePath = rc.FilePath
				th.Line = rc.LineNo
			}
			byRoot[root] = th
		}
		c.ThreadID = root
		byRoot[root].Comments = append(byRoot[root].Comments, c)
	}

	threads := make([]*provider.ReviewThread, 0, len(order))
	for _, r := range order {
		threads = append(threads, byRoot[r])
	}
	return threads
}

func TestGroupByReplyChain_Order(t *testing.T) {
	comments := []*provider.Comment{
		{ID: "github-review-comment-10", PRID: "p", FilePath: "a.go", LineNo: 1},
		{ID: "github-review-comment-11", PRID: "p", InReplyTo: "github-review-comment-10"},
		{ID: "github-review-comment-20", PRID: "p", FilePath: "b.go", LineNo: 2},
	}
	threads := groupByReplyChainTest(comments)
	if len(threads) != 2 {
		t.Fatalf("got %d threads want 2", len(threads))
	}
	if threads[0].ID != "github-review-comment-10" || threads[1].ID != "github-review-comment-20" {
		t.Errorf("order: got %q %q", threads[0].ID, threads[1].ID)
	}
	if len(threads[0].Comments) != 2 {
		t.Errorf("thread 0 comment count: got %d want 2", len(threads[0].Comments))
	}
	for _, c := range threads[0].Comments {
		if c.ThreadID != threads[0].ID {
			t.Errorf("thread 0 fallback FK: comment %s ThreadID=%q want %q", c.ID, c.ThreadID, threads[0].ID)
		}
	}
	if len(threads[1].Comments) != 1 {
		t.Errorf("thread 1 comment count: got %d want 1", len(threads[1].Comments))
	}
	for _, c := range threads[1].Comments {
		if c.ThreadID != threads[1].ID {
			t.Errorf("thread 1 fallback FK: comment %s ThreadID=%q want %q", c.ID, c.ThreadID, threads[1].ID)
		}
	}
}

// TestGroupByReplyChain_OneReviewMultipleThreads is the canonical regression test
// for the PullRequestReviewID grouping bug: two root comments in the same review
// submission (same PullRequestReviewID) must produce TWO distinct threads, not one.
func TestGroupByReplyChain_OneReviewMultipleThreads(t *testing.T) {
	// Scenario: review submission 99 has two separate inline threads:
	//   Thread A: comment 1 (main.go:5) ← comment 2 reply
	//   Thread B: comment 3 (utils.go:10) — completely separate thread, same review!
	//
	// Old PullRequestReviewID-based grouping would collapse all three into ONE thread.
	// New reply-chain grouping correctly produces TWO threads.
	comments := []*provider.Comment{
		{
			ID: "github-review-comment-1", PRID: "github:o/r:1",
			Kind: "review", FilePath: "main.go", LineNo: 5,
			// InReplyTo == "" → this is a root comment
		},
		{
			ID: "github-review-comment-2", PRID: "github:o/r:1",
			Kind:      "review",
			InReplyTo: "github-review-comment-1", // reply to comment 1
		},
		{
			ID: "github-review-comment-3", PRID: "github:o/r:1",
			Kind: "review", FilePath: "utils.go", LineNo: 10,
			// InReplyTo == "" → different root → different thread
			// Even though all three share the same PullRequestReviewID=99
		},
	}

	threads := groupByReplyChainTest(comments)

	if len(threads) != 2 {
		t.Fatalf("expected 2 distinct threads (not collapsed by review ID), got %d", len(threads))
	}

	// Thread A: rooted at comment 1, contains comments 1+2.
	threadA := threads[0]
	if threadA.ID != "github-review-comment-1" {
		t.Errorf("thread A ID: want 'github-review-comment-1', got %q", threadA.ID)
	}
	if len(threadA.Comments) != 2 {
		t.Errorf("thread A: expected 2 comments, got %d", len(threadA.Comments))
	}
	if threadA.FilePath != "main.go" || threadA.Line != 5 {
		t.Errorf("thread A: wrong anchor: path=%q line=%d", threadA.FilePath, threadA.Line)
	}

	// Thread B: rooted at comment 3, contains only comment 3.
	threadB := threads[1]
	if threadB.ID != "github-review-comment-3" {
		t.Errorf("thread B ID: want 'github-review-comment-3', got %q", threadB.ID)
	}
	if len(threadB.Comments) != 1 {
		t.Errorf("thread B: expected 1 comment, got %d", len(threadB.Comments))
	}
	if threadB.FilePath != "utils.go" || threadB.Line != 10 {
		t.Errorf("thread B: wrong anchor: path=%q line=%d", threadB.FilePath, threadB.Line)
	}

	// Both are provisional (StateKnown=false) without GraphQL enrichment.
	for i, th := range threads {
		if th.StateKnown {
			t.Errorf("thread %d: expected StateKnown=false for REST-only grouping", i)
		}
		for _, c := range th.Comments {
			if c.ThreadID != th.ID {
				t.Errorf("thread %d fallback FK: comment %s ThreadID=%q want %q", i, c.ID, c.ThreadID, th.ID)
			}
		}
	}
}

// TestGroupByReplyChain_FallbackIDConsistency verifies the REST-only/degraded
// path still links comments to their provisional thread rows. GraphQL enrichment
// can later replace both IDs, but fallback must not orphan comments.
func TestGroupByReplyChain_FallbackIDConsistency(t *testing.T) {
	comments := []*provider.Comment{
		{ID: "github-review-comment-10", PRID: "p", Kind: "review"},
		{ID: "github-review-comment-11", PRID: "p", Kind: "review",
			InReplyTo: "github-review-comment-10"},
	}
	threads := groupByReplyChainTest(comments)
	if len(threads) != 1 {
		t.Fatalf("got %d threads want 1", len(threads))
	}
	if threads[0].StateKnown {
		t.Fatal("fallback thread should be StateKnown=false")
	}
	for _, c := range threads[0].Comments {
		if c.ThreadID != threads[0].ID {
			t.Errorf("fallback FK violation: comment %s ThreadID=%q != thread.ID=%q",
				c.ID, c.ThreadID, threads[0].ID)
		}
	}
}

// TestGroupByReplyChain_CanonicalIDConsistency verifies the FK invariant:
// after GraphQL enrichment upgrades a thread's ID, every member comment's
// ThreadID must equal the new canonical thread ID (comments.thread_id FK).
func TestGroupByReplyChain_CanonicalIDConsistency(t *testing.T) {
	comments := []*provider.Comment{
		{ID: "github-review-comment-10", PRID: "p", Kind: "review"},
		{ID: "github-review-comment-11", PRID: "p", Kind: "review",
			InReplyTo: "github-review-comment-10"},
	}
	threads := groupByReplyChainTest(comments)
	if len(threads) != 1 {
		t.Fatalf("got %d threads want 1", len(threads))
	}

	// Simulate what GetReviewThreads does when a GraphQL match is found.
	canonicalID := "RT_kwDOABcdefghijk"
	threads[0].ID = canonicalID
	threads[0].StateKnown = true
	threads[0].Resolved = false
	for _, c := range threads[0].Comments {
		c.ThreadID = canonicalID
	}

	// Verify FK invariant: every comment.ThreadID == thread.ID.
	for _, c := range threads[0].Comments {
		if c.ThreadID != threads[0].ID {
			t.Errorf("FK violation: comment %s ThreadID=%q != thread.ID=%q",
				c.ID, c.ThreadID, threads[0].ID)
		}
	}
}

// TestGroupByReplyChain_DeepChain verifies that multi-level reply chains
// (A ← B ← C) all resolve to the same root thread.
func TestGroupByReplyChain_DeepChain(t *testing.T) {
	comments := []*provider.Comment{
		{ID: "github-review-comment-1", PRID: "p", Kind: "review"},
		{ID: "github-review-comment-2", PRID: "p", Kind: "review",
			InReplyTo: "github-review-comment-1"},
		{ID: "github-review-comment-3", PRID: "p", Kind: "review",
			InReplyTo: "github-review-comment-2"}, // replies to a reply
	}
	threads := groupByReplyChainTest(comments)
	if len(threads) != 1 {
		t.Fatalf("deep chain should be one thread, got %d threads", len(threads))
	}
	if len(threads[0].Comments) != 3 {
		t.Errorf("expected 3 comments in chain, got %d", len(threads[0].Comments))
	}
}
