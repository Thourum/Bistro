package prsync

import (
	"context"
	"fmt"
	"strings"
	gosync "sync"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/store/reviewdraft"
)

// reviewDraftStore is the subset of reviewdraft.Store review submission uses.
type reviewDraftStore interface {
	List(ctx context.Context, prID, login string) ([]reviewdraft.Draft, error)
	Delete(ctx context.Context, login string, ids []string) error
}

// draftGroup holds drafts submitted together: GitHub anchors every comment of
// a review to the review's single commit_id.
type draftGroup struct {
	commitSHA string
	drafts    []reviewdraft.Draft
}

// applyReviewWithDrafts handles approve, request_changes and submit_review when
// drafts are involved. handled=false means ApplyAction continues normally:
// the request is not a review, or a plain approve/request_changes has no drafts.
func (o *Orchestrator) applyReviewWithDrafts(ctx context.Context, req *actions.ActionRequest) (result *actions.ActionResult, handled bool) {
	switch req.Action {
	case actions.ApproveCommand, actions.RequestChangesCommand, actions.SubmitReviewCommand:
	default:
		return nil, false
	}
	isExplicitSubmit := req.Action == actions.SubmitReviewCommand
	if o.drafts == nil || o.reviewLogin == nil {
		if isExplicitSubmit {
			return failedActionResult(req, "review drafts are unavailable"), true
		}
		return nil, false
	}
	login, ok, err := o.reviewLogin.Get(ctx, provider.GitHub)
	if err != nil {
		return failedActionResult(req, fmt.Sprintf("resolve current user for review: %v", err)), true
	}
	if !ok || login == "" {
		if isExplicitSubmit {
			return failedActionResult(req, "no current GitHub account: review not submitted"), true
		}
		return nil, false
	}
	lock, _ := o.reviewLocks.LoadOrStore(req.PRID, &gosync.Mutex{})
	mu := lock.(*gosync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	drafts, err := o.drafts.List(ctx, req.PRID, login)
	if err != nil {
		return failedActionResult(req, fmt.Sprintf("load review drafts: %v", err)), true
	}
	if len(drafts) == 0 && !isExplicitSubmit {
		return nil, false
	}
	return o.submitReview(ctx, asSubmitReview(req), login, drafts), true
}

// asSubmitReview turns approve/request_changes into the equivalent submit_review.
func asSubmitReview(req *actions.ActionRequest) *actions.ActionRequest {
	converted := *req
	switch req.Action {
	case actions.ApproveCommand:
		converted.ReviewEvent = actions.ReviewEventApprove
	case actions.RequestChangesCommand:
		converted.ReviewEvent = actions.ReviewEventRequestChanges
	}
	converted.Action = actions.SubmitReviewCommand
	return &converted
}

// submitReview sends one review per commit group, oldest groups first as
// COMMENT reviews, and deletes each group's drafts once GitHub accepts it.
// Never queued: drafts are already persisted, so failure just keeps them.
func (o *Orchestrator) submitReview(ctx context.Context, req *actions.ActionRequest, login string, drafts []reviewdraft.Draft) *actions.ActionResult {
	// Reject a bad event or body before older commit groups post.
	if err := req.Validate(); err != nil {
		return failedActionResult(req, fmt.Sprintf("validation failed: %v", err))
	}
	if len(drafts) == 0 && req.ReviewEvent == actions.ReviewEventComment && req.Body == "" {
		return failedActionResult(req, "nothing to submit: no drafts and no review body")
	}
	if o.IsRateLimited() {
		return failedActionResult(req, fmt.Sprintf("rate limited: review not submitted; %d draft(s) kept", len(drafts)))
	}
	headSHA := o.prHeadSHA(ctx, req.PRID)
	groups := groupDraftsByCommit(drafts, headSHA)
	if len(groups) == 0 {
		groups = []draftGroup{{}} // body-only review
	} else if headSHA != "" && req.ReviewEvent != actions.ReviewEventComment && groups[len(groups)-1].commitSHA != headSHA {
		// Approval and change requests must target head even with only older drafts.
		groups = append(groups, draftGroup{commitSHA: headSHA})
	}
	var result *actions.ActionResult
	for i, group := range groups {
		result = o.executeAction(ctx, reviewRequestForGroup(req, group, i == len(groups)-1))
		if !result.Success {
			if result.IsAmbiguousRemoteSuccess() {
				return ambiguousReviewResult(req, groups[i:], result.Error)
			}
			if i == 0 && len(group.drafts) == 0 {
				return result
			}
			// Groups before i were accepted and their drafts deleted, so resubmitting
			// sends only what is kept.
			msg := "review not submitted"
			if i > 0 {
				msg = "comments on older commits posted; " + msg
			}
			if len(group.drafts) > 0 {
				msg += fmt.Sprintf("; %d draft(s) kept (%s)", len(group.drafts), describeDrafts(group.drafts))
			}
			return failedActionResult(req, fmt.Sprintf("%s: %s", msg, result.Error))
		}
		o.deleteSubmittedDrafts(ctx, req.PRID, login, group.drafts)
	}
	return result
}

// ambiguousReviewResult keeps the ambiguous sentinel so the UI warns before a
// resubmit, which could post the same comments twice, and names what is left.
func ambiguousReviewResult(req *actions.ActionRequest, unsent []draftGroup, cause string) *actions.ActionResult {
	group, unsentGroups := unsent[0], len(unsent)-1
	msg := actions.ErrAmbiguousRemoteSuccess + "review may have been submitted; check GitHub before resubmitting."
	if len(group.drafts) > 0 {
		msg += fmt.Sprintf(" %d draft(s) kept (%s).", len(group.drafts), describeDrafts(group.drafts))
	}
	if unsentGroups > 0 {
		msg += fmt.Sprintf(" %d later commit group(s) not sent.", unsentGroups)
	}
	msg += fmt.Sprintf(" (cause: %s)", strings.TrimPrefix(cause, actions.ErrAmbiguousRemoteSuccess))
	return failedActionResult(req, msg)
}

// groupDraftsByCommit keeps first-seen order and moves the head commit's group
// last, so the review carrying the user's event lands on the current head.
func groupDraftsByCommit(drafts []reviewdraft.Draft, headSHA string) []draftGroup {
	var groups []draftGroup
	indexBySHA := map[string]int{}
	for _, draft := range drafts {
		i, ok := indexBySHA[draft.CommitSHA]
		if !ok {
			i = len(groups)
			indexBySHA[draft.CommitSHA] = i
			groups = append(groups, draftGroup{commitSHA: draft.CommitSHA})
		}
		groups[i].drafts = append(groups[i].drafts, draft)
	}
	if i, ok := indexBySHA[headSHA]; ok && i != len(groups)-1 {
		head := groups[i]
		groups = append(append(groups[:i:i], groups[i+1:]...), head)
	}
	return groups
}

// reviewRequestForGroup builds one CreateReview request. Only the last group
// carries the user's event and body; earlier groups are plain COMMENT reviews.
func reviewRequestForGroup(req *actions.ActionRequest, group draftGroup, isLast bool) *actions.ActionRequest {
	groupReq := &actions.ActionRequest{
		PRID: req.PRID, Action: actions.SubmitReviewCommand,
		ReviewEvent: actions.ReviewEventComment, CommitID: group.commitSHA,
		ReviewComments: toReviewComments(group.drafts),
	}
	if isLast {
		groupReq.ReviewEvent = req.ReviewEvent
		groupReq.Body = req.Body
	}
	return groupReq
}

func toReviewComments(drafts []reviewdraft.Draft) []provider.ReviewComment {
	comments := make([]provider.ReviewComment, 0, len(drafts))
	for _, draft := range drafts {
		comments = append(comments, provider.ReviewComment{
			Path: draft.Path, Side: draft.Side, StartLine: draft.StartLine, Line: draft.Line, Body: draft.Body,
		})
	}
	return comments
}

// describeDrafts names drafts as path:line or path:start-end for error messages.
func describeDrafts(drafts []reviewdraft.Draft) string {
	names := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		if draft.StartLine > 0 {
			names = append(names, fmt.Sprintf("%s:%d-%d", draft.Path, draft.StartLine, draft.Line))
			continue
		}
		names = append(names, fmt.Sprintf("%s:%d", draft.Path, draft.Line))
	}
	return strings.Join(names, ", ")
}

// prHeadSHA returns the cached head SHA, or "" when unknown (no reordering).
func (o *Orchestrator) prHeadSHA(ctx context.Context, prID string) string {
	if o.store == nil {
		return ""
	}
	var headSHA string
	if err := o.store.QueryRowContext(ctx, `SELECT head_sha FROM prs WHERE id = ?`, prID).Scan(&headSHA); err != nil {
		logging.FromContext(ctx).Debug("prHeadSHA: lookup failed", "pr_id", prID, "error", err)
		return ""
	}
	return headSHA
}

// deleteSubmittedDrafts removes drafts GitHub accepted and tells the UI.
func (o *Orchestrator) deleteSubmittedDrafts(ctx context.Context, prID, login string, drafts []reviewdraft.Draft) {
	if len(drafts) == 0 {
		return
	}
	ids := make([]string, 0, len(drafts))
	for _, draft := range drafts {
		ids = append(ids, draft.ID)
	}
	if err := o.drafts.Delete(ctx, login, ids); err != nil {
		// The review is on GitHub; resubmitting these drafts would duplicate it.
		logging.FromContext(ctx).Error("submitted drafts could not be deleted", "pr_id", prID, "count", len(ids), "error", err)
	}
	if o.emitter != nil {
		o.emitter.Emit(&events.Event{
			Type: "DRAFTS_CHANGED", Data: map[string]interface{}{"pr_id": prID}, Timestamp: time.Now(),
		})
	}
}
