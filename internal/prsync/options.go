package prsync

import (
	"context"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/classify"
	"github.com/itterno/bistro/internal/git/anchor"
	"github.com/itterno/bistro/internal/prsync/writer"
	"github.com/itterno/bistro/internal/store/actionqueue"
)

// WithReviewTracking attaches login-resolver and SHA-writer so that successful
// review actions (approve, request-changes, comment) record last_reviewed_sha.
func (o *Orchestrator) WithReviewTracking(loginGetter actions.ReviewLoginGetter, shaWriter actions.ReviewSHAWriter) *Orchestrator {
	o.reviewLogin = loginGetter
	o.reviewSHAWriter = shaWriter
	return o
}

// WithAnchorResolver attaches an anchor.Resolver so inline review comment
// anchors are resolved and cached after each successful diff clone.
func (o *Orchestrator) WithAnchorResolver(r *anchor.Resolver) *Orchestrator {
	o.anchorResolver = r
	return o
}

// WithActionQueue attaches a persistent action queue for offline/rate-limited
// action replay. Production orchestrators should always have a queue attached.
func (o *Orchestrator) WithActionQueue(q *actionqueue.Queue) *Orchestrator {
	o.queue = q
	return o
}

// WithLiveness attaches a context that is cancelled when this orchestrator is
// superseded (provider disconnected or replaced). ApplyAction checks it before
// executing so a handler that captured a stale orchestrator reference before
// disconnect will have its action rejected rather than executed with old credentials.
func (o *Orchestrator) WithLiveness(ctx context.Context) *Orchestrator {
	o.liveness = ctx
	return o
}

// WithClassifier attaches the classification engine provider; writers built
// by this orchestrator classify comments in the write path.
func (o *Orchestrator) WithClassifier(p *classify.EngineProvider) *Orchestrator {
	o.classifier = p
	return o
}

// WithCurrentUser attaches a current-user reader so sync writers can detect
// mentions of the authenticated GitHub account.
func (o *Orchestrator) WithCurrentUser(reader writer.CurrentUserReader) *Orchestrator {
	o.currentUser = reader
	return o
}

// WithExpertise attaches the expertise engine hook for post-diff incremental updates.
func (o *Orchestrator) WithExpertise(h ExpertiseHook) *Orchestrator {
	o.expertise = h
	return o
}

// WithRepoPathResolver attaches local checkout path resolution for expertise updates.
func (o *Orchestrator) WithRepoPathResolver(resolve RepoPathResolver) *Orchestrator {
	o.resolveRepoPath = resolve
	return o
}

// WithPostCommit attaches a hook writers invoke after each committed batch
// with the written PR ids.
func (o *Orchestrator) WithPostCommit(fn func(prIDs []string)) *Orchestrator {
	o.postCommit = fn
	return o
}

// WithPostReview attaches a hook invoked after successful review-like actions.
func (o *Orchestrator) WithPostReview(fn func(ctx context.Context, prID string)) *Orchestrator {
	o.postReview = fn
	return o
}

// WithReviewDrafts attaches the draft store used for review submission.
func (o *Orchestrator) WithReviewDrafts(d reviewDraftStore) *Orchestrator {
	o.drafts = d
	return o
}
