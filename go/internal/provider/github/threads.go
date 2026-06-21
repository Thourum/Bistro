package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
	"github.com/shurcooL/githubv4"
)

// NormalizeReviewComment converts a GitHub PullRequestComment (inline review
// comment) to the domain Comment type with Kind="review" and anchoring fields.
//
// ThreadID is intentionally left empty here. It is assigned by GetReviewThreads
// after grouping: canonical GraphQL node IDs for known-state threads, or the
// provisional root-comment domain ID for REST-only fallback threads.
// PullRequestReviewID is NOT used as a thread identifier because one review
// submission may span several distinct inline threads on different files/lines.
func NormalizeReviewComment(prID string, ghc *ghlib.PullRequestComment) *provider.Comment {
	c := &provider.Comment{
		ID:          "github-review-comment-" + strconv.FormatInt(ghc.GetID(), 10),
		PRID:        prID,
		Author:      ghc.GetUser().GetLogin(),
		Body:        ghc.GetBody(),
		Kind:        "review",
		CommitSHA:   ghc.GetCommitID(),
		OriginalSHA: ghc.GetOriginalCommitID(),
		Side:        ghc.GetSide(),
		FilePath:    ghc.GetPath(),
		LineNo:      ghc.GetLine(),
		DiffHunk:    ghc.GetDiffHunk(),
		CreatedAt:   ghc.GetCreatedAt().Time,
		UpdatedAt:   ghc.GetUpdatedAt().Time,
	}

	// Position: 0 means the comment is on an outdated diff hunk — treat as nil.
	if p := ghc.GetPosition(); p != 0 {
		c.Position = &p
	}
	if op := ghc.GetOriginalPosition(); op != 0 {
		c.OriginalPosition = &op
	}

	// InReplyTo: build the canonical ID from the numeric parent ID.
	if rid := ghc.GetInReplyTo(); rid != 0 {
		c.InReplyTo = "github-review-comment-" + strconv.FormatInt(rid, 10)
	}

	// ThreadID intentionally not set here; assigned after grouping.
	return c
}

// listReviewComments fetches all inline PR review comments via REST.
func (c *Client) listReviewComments(
	ctx context.Context, owner, repo string, number int,
) ([]*provider.Comment, error) {
	opts := &ghlib.PullRequestListCommentsOptions{
		ListOptions: ghlib.ListOptions{PerPage: 100},
	}
	prID := "github:" + owner + "/" + repo + ":" + strconv.Itoa(number)
	var comments []*provider.Comment
	for {
		ghComments, resp, err := c.gh.PullRequests.ListComments(ctx, owner, repo, number, opts)
		if err != nil {
			return nil, fmt.Errorf("list review comments for %s/%s#%d: %w", owner, repo, number, err)
		}
		for _, ghc := range ghComments {
			comments = append(comments, NormalizeReviewComment(prID, ghc))
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return comments, nil
}

// graphqlThreadInfo carries everything needed from one GraphQL reviewThread node:
// the canonical node ID, resolution state, and the REST database IDs of its
// comments — which are the cross-reference key to REST PullRequestComment.ID.
type graphqlThreadInfo struct {
	NodeID       string // canonical GraphQL thread node ID (formatted from githubv4.ID)
	IsResolved   bool
	IsOutdated   bool
	CommentDBIDs []int64 // REST numeric IDs of comments in this thread
}

// fetchGraphQLThreads queries the GitHub GraphQL API for review thread state.
// Each thread node includes the databaseId of every member comment so callers
// can match REST comments to canonical thread node IDs by database ID identity
// rather than by fragile list-position heuristics.
//
// Returns nil, nil on any GraphQL error — callers should fall back to the
// REST-only provisional grouping with StateKnown=false.
func (c *Client) fetchGraphQLThreads(ctx context.Context, owner, repo string, number int) ([]graphqlThreadInfo, error) {
	var q struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []struct {
						ID         githubv4.ID `graphql:"id"`
						IsResolved bool        `graphql:"isResolved"`
						IsOutdated bool        `graphql:"isOutdated"`
						Comments   struct {
							Nodes []struct {
								DatabaseId int64 `graphql:"databaseId"`
							}
						} `graphql:"comments(first: 100)"`
					}
				} `graphql:"reviewThreads(first: 100)"`
			} `graphql:"pullRequest(number: $number)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	vars := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"name":   githubv4.String(repo),
		"number": githubv4.Int(number),
	}

	if err := c.gqlClient.Query(ctx, &q, vars); err != nil {
		return nil, err
	}

	out := make([]graphqlThreadInfo, 0, len(q.Repository.PullRequest.ReviewThreads.Nodes))
	for _, n := range q.Repository.PullRequest.ReviewThreads.Nodes {
		nodeID := fmt.Sprintf("%v", n.ID)
		if nodeID == "" || nodeID == "<nil>" {
			continue
		}
		info := graphqlThreadInfo{
			NodeID:     nodeID,
			IsResolved: n.IsResolved,
			IsOutdated: n.IsOutdated,
		}
		for _, nc := range n.Comments.Nodes {
			if nc.DatabaseId != 0 {
				info.CommentDBIDs = append(info.CommentDBIDs, nc.DatabaseId)
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// GetReviewThreads implements provider.ThreadProvider.
//
// Identity contract:
//   - Known-state threads (GraphQL enrichment succeeded): thread.ID and every
//     child comment's ThreadID equal the canonical GraphQL review-thread node ID.
//     This ID is suitable as the provider-opaque handle for 2.4 resolve/unresolve.
//   - Unknown-state threads (GraphQL failed or comment not matched): thread.ID
//     is the provisional root-comment domain ID; ThreadID on comments equals the
//     same value. StateKnown=false. No canonical GraphQL node ID is claimed.
//
// Algorithm:
//  1. REST: fetch all inline comments; group into provisional threads by
//     in_reply_to reply chains (each root = one thread). StateKnown=false.
//  2. GraphQL: fetch thread nodes with each member comment's databaseId.
//     Build map: REST comment numeric ID → canonical thread node ID + state.
//  3. Enrichment: for each provisional thread, check any member comment's
//     numeric ID against the map; on first match, upgrade thread.ID to the
//     canonical GraphQL node ID, propagate to all member comments' ThreadID,
//     and set Resolved/Outdated/StateKnown=true.
//  4. If GraphQL fails: return provisional threads unchanged, no error.
//  5. If REST fails: return genuine error (comment content unavailable).
func (c *Client) GetReviewThreads(ctx context.Context, prID string) ([]*provider.ReviewThread, error) {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return nil, err
	}

	// Step 1: REST — authoritative source for comment content and reply structure.
	comments, err := c.listReviewComments(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("GetReviewThreads REST: %w", err)
	}

	threads := groupByReplyChain(comments)

	// Step 2: GraphQL enrichment — on any failure, return REST threads as-is.
	gqlThreads, gqlErr := c.fetchGraphQLThreads(ctx, owner, repo, number)
	if gqlErr != nil {
		return threads, nil
	}

	// Build cross-reference: REST comment numeric ID → graphqlThreadInfo.
	type enrichKey struct {
		nodeID     string
		isResolved bool
		isOutdated bool
	}
	commentToThread := make(map[int64]enrichKey, len(gqlThreads)*4)
	for _, gt := range gqlThreads {
		k := enrichKey{gt.NodeID, gt.IsResolved, gt.IsOutdated}
		for _, dbID := range gt.CommentDBIDs {
			commentToThread[dbID] = k
		}
	}

	// Step 3: Enrich each provisional thread that has at least one identifiable comment.
	for _, th := range threads {
		var matched *enrichKey
		for _, com := range th.Comments {
			// Parse the raw numeric REST ID from the domain comment ID.
			dbID := restDBIDFromDomainID(com.ID)
			if dbID <= 0 {
				continue
			}
			if m, ok := commentToThread[dbID]; ok {
				matched = &m
				break
			}
		}
		if matched == nil {
			// No GraphQL match: keep provisional ID and StateKnown=false.
			continue
		}
		// Upgrade: canonical GraphQL node ID becomes the thread's stable identity.
		// Propagate to every member comment so comments.thread_id FK is consistent.
		th.ID = matched.nodeID
		th.Resolved = matched.isResolved
		th.Outdated = matched.isOutdated
		th.StateKnown = true
		for _, com := range th.Comments {
			com.ThreadID = matched.nodeID
		}
	}

	return threads, nil
}

// groupByReplyChain groups inline review comments into provisional review threads
// using the in_reply_to relationship: each root comment (InReplyTo=="") starts a
// new thread; replies are attached to the same thread as their ultimate root.
//
// This correctly separates multiple threads within a single review submission
// (PullRequestReviewID), which the old PullRequestReviewID-based grouping could not.
// StateKnown=false because this is REST-only; GetReviewThreads upgrades matched
// threads to StateKnown=true after GraphQL enrichment.
func groupByReplyChain(comments []*provider.Comment) []*provider.ReviewThread {
	// Index by domain comment ID for O(1) parent lookups.
	byID := make(map[string]*provider.Comment, len(comments))
	for _, c := range comments {
		byID[c.ID] = c
	}

	// Memoised root-finder: trace InReplyTo links up to the root comment.
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
				// Provisional ID: root comment's domain ID.
				// Replaced with canonical GraphQL node ID on enrichment.
				ID:         root,
				PRID:       c.PRID,
				StateKnown: false,
			}
			if rootComment, ok := byID[root]; ok {
				th.FilePath = rootComment.FilePath
				th.Line = rootComment.LineNo
			}
			byRoot[root] = th
		}
		// Provisional/fallback threads must still maintain the local FK invariant:
		// comments.thread_id references review_threads.id. GraphQL enrichment may
		// later overwrite both with the canonical provider thread node ID.
		c.ThreadID = root
		byRoot[root].Comments = append(byRoot[root].Comments, c)
	}

	threads := make([]*provider.ReviewThread, 0, len(order))
	for _, r := range order {
		threads = append(threads, byRoot[r])
	}
	return threads
}

// restDBIDFromDomainID parses the numeric REST database ID from a domain
// comment ID of the form "github-review-comment-<N>".
// Returns 0 if the ID does not match this shape.
func restDBIDFromDomainID(domainID string) int64 {
	const prefix = "github-review-comment-"
	if !strings.HasPrefix(domainID, prefix) {
		return 0
	}
	n, err := strconv.ParseInt(domainID[len(prefix):], 10, 64)
	if err != nil {
		return 0
	}
	return n
}
