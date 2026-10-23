package sync

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
)

type reviewRecorder struct {
	*mock.MockProvider
	submissions []provider.ReviewSubmission
	err         error
	failAfter   int
	calls       int
}

func (r *reviewRecorder) SubmitReview(_ context.Context, _ string, sub provider.ReviewSubmission) error {
	r.calls++
	if r.err != nil && len(r.submissions) >= r.failAfter {
		return r.err
	}
	r.submissions = append(r.submissions, sub)
	return nil
}

type staticLogin string

func (s staticLogin) GetCurrentUser(context.Context, string) (string, bool, error) {
	return string(s), true, nil
}

func newReviewTestOrchestrator(t *testing.T, prov provider.Provider) (*Orchestrator, *store.ReviewDrafts) {
	t.Helper()
	db := openTestDB(t)
	insertPRForSync(t, db.DB, "pr-r") // head_sha = 'head'
	drafts := store.NewReviewDrafts(db.DB)
	orch := NewOrchestrator(nil, prov, db, nil).
		WithReviewTracking(staticLogin("me"), db).
		WithReviewDrafts(drafts)
	return orch, drafts
}

func mustDraft(t *testing.T, drafts *store.ReviewDrafts, draft store.ReviewDraft) store.ReviewDraft {
	t.Helper()
	draft.PRID = "pr-r"
	created, err := drafts.Create(context.Background(), "me", draft)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	return created
}

func TestApplyAction_ApproveAbsorbsDraftsIntoOneReview(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
	mustDraft(t, drafts, store.ReviewDraft{Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "two", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.ApproveCommand})

	if !result.Success {
		t.Fatalf("result = %+v", result)
	}
	if len(prov.submissions) != 1 {
		t.Fatalf("submissions = %d, want 1", len(prov.submissions))
	}
	got := prov.submissions[0]
	if got.Event != actions.ReviewEventApprove || got.CommitID != "head" || len(got.Comments) != 2 {
		t.Fatalf("submission = %+v", got)
	}
	// Drafts created in the same second are ordered by random id, not insertion.
	for _, comment := range []provider.ReviewComment{
		{Path: "a.go", Side: "RIGHT", Line: 10, Body: "one"},
		{Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "two"},
	} {
		if !slices.Contains(got.Comments, comment) {
			t.Fatalf("comments = %+v, missing %+v", got.Comments, comment)
		}
	}
	left, err := drafts.List(context.Background(), "pr-r", "me")
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	if len(left) != 0 {
		t.Fatalf("drafts left = %d, want submitted drafts deleted", len(left))
	}
}

func TestApplyAction_DraftSuccessRefreshesAndPreservesOtherAccounts(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mine := mustDraft(t, drafts, store.ReviewDraft{Path: "mine.go", Side: "RIGHT", Line: 1, Body: "mine", CommitSHA: "head"})
	other, err := drafts.Create(context.Background(), "other", store.ReviewDraft{PRID: "pr-r", Path: "other.go", Side: "RIGHT", Line: 1, Body: "other", CommitSHA: "head"})
	if err != nil {
		t.Fatalf("create other draft: %v", err)
	}
	eventChannel := orch.GetEventChannel()
	defer orch.UnsubscribeEventChannel(eventChannel)
	var refreshedPR string
	orch.WithPostReview(func(_ context.Context, prID string) { refreshedPR = prID })
	request := &actions.ActionRequest{PRID: "pr-r", Action: actions.ApproveCommand}

	result := orch.ApplyAction(context.Background(), request)

	if !result.Success || refreshedPR != "pr-r" || request.Action != actions.ApproveCommand {
		t.Fatalf("result = %+v, refreshedPR = %q, request = %+v", result, refreshedPR, request)
	}
	if len(prov.submissions) != 1 || len(prov.submissions[0].Comments) != 1 || prov.submissions[0].Comments[0].Path != mine.Path {
		t.Fatalf("submissions = %+v, want only current user's draft", prov.submissions)
	}
	remaining, err := drafts.List(context.Background(), "pr-r", "other")
	if err != nil || len(remaining) != 1 || remaining[0].ID != other.ID {
		t.Fatalf("other account drafts = %+v, err = %v", remaining, err)
	}
	for {
		select {
		case event := <-eventChannel:
			if event.Type == "DRAFTS_CHANGED" {
				if event.Data["pr_id"] != "pr-r" || event.Timestamp.IsZero() {
					t.Fatalf("draft event = %+v", event)
				}
				return
			}
		default:
			t.Fatal("missing DRAFTS_CHANGED event after deleting submitted drafts")
		}
	}
}

func TestApplyAction_FailedSubmitKeepsDraftsAndNamesThem(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider(), err: errors.New("POST .../reviews: 422 Unprocessable Entity")}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 10, Body: "stale", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.SubmitReviewCommand, ReviewEvent: actions.ReviewEventComment})

	if result.Success || !strings.Contains(result.Error, "a.go:10") || !strings.Contains(result.Error, "422") {
		t.Fatalf("result = %+v, want a failure naming a.go:10 and the 422", result)
	}
	left, err := drafts.List(context.Background(), "pr-r", "me")
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	if len(left) != 1 {
		t.Fatalf("drafts left = %d, want the draft kept", len(left))
	}
}

func TestApplyAction_DraftsOnOlderCommitGoFirstAsCommentReview(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "new.go", Side: "RIGHT", Line: 1, Body: "on head", CommitSHA: "head"})
	mustDraft(t, drafts, store.ReviewDraft{Path: "old.go", Side: "RIGHT", Line: 2, Body: "outdated", CommitSHA: "old"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.RequestChangesCommand, Body: "Please fix"})

	if !result.Success || len(prov.submissions) != 2 {
		t.Fatalf("result = %+v, submissions = %d", result, len(prov.submissions))
	}
	first, last := prov.submissions[0], prov.submissions[1]
	if first.CommitID != "old" || first.Event != actions.ReviewEventComment || first.Body != "" {
		t.Fatalf("first = %+v, want a body-less COMMENT review on the old commit", first)
	}
	if last.CommitID != "head" || last.Event != actions.ReviewEventRequestChanges || last.Body != "Please fix" {
		t.Fatalf("last = %+v, want the user's event and body on head", last)
	}
}

func TestApplyAction_RateLimitedReviewIsNotQueued(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	queue := store.NewActionQueue(orch.store.DB)
	orch.WithActionQueue(queue)
	orch.rateLimitState = &provider.RateLimitState{ResetAt: time.Now().Add(5 * time.Minute)}
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 1, Body: "x", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.ApproveCommand})

	if result.Success || !strings.Contains(result.Error, "draft(s) kept") || len(prov.submissions) != 0 {
		t.Fatalf("result = %+v, submissions = %d", result, len(prov.submissions))
	}
	left, err := drafts.List(context.Background(), "pr-r", "me")
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	if len(left) != 1 {
		t.Fatalf("drafts left = %d, want 1", len(left))
	}
	pending, err := queue.GetPending(context.Background(), 1)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending = %+v, err = %v, want no queued submission", pending, err)
	}
}

func TestApplyAction_DraftsOnlyOnOlderCommitApproveHeadSeparately(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "old.go", Side: "RIGHT", Line: 2, Body: "outdated", CommitSHA: "old"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.ApproveCommand})

	if !result.Success || len(prov.submissions) != 2 {
		t.Fatalf("result = %+v, submissions = %d", result, len(prov.submissions))
	}
	first, last := prov.submissions[0], prov.submissions[1]
	if first.CommitID != "old" || first.Event != actions.ReviewEventComment || first.Body != "" || len(first.Comments) != 1 {
		t.Fatalf("first = %+v, want a body-less COMMENT review with the old draft", first)
	}
	if last.CommitID != "head" || last.Event != actions.ReviewEventApprove || len(last.Comments) != 0 {
		t.Fatalf("last = %+v, want a draft-less APPROVE review on head", last)
	}
	left, err := drafts.List(context.Background(), "pr-r", "me")
	if err != nil || len(left) != 0 {
		t.Fatalf("drafts left = %+v, err = %v, want none", left, err)
	}
}

func TestApplyAction_ReviewPartialSuccessDeletesOnlyAcceptedGroup(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider(), err: errors.New("422 Unprocessable Entity"), failAfter: 1}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "old.go", Side: "RIGHT", Line: 1, Body: "old", CommitSHA: "old"})
	rejected := mustDraft(t, drafts, store.ReviewDraft{Path: "head.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "stale", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.RequestChangesCommand, Body: "Please fix"})

	if result.Success || !strings.Contains(result.Error, "head.go:3-5") || !strings.Contains(result.Error, "422") {
		t.Fatalf("result = %+v, want failed range draft identified", result)
	}
	if len(prov.submissions) != 1 || prov.submissions[0].CommitID != "old" {
		t.Fatalf("submissions = %+v, want only the accepted old group", prov.submissions)
	}
	remaining, err := drafts.List(context.Background(), "pr-r", "me")
	if err != nil || len(remaining) != 1 || remaining[0].ID != rejected.ID {
		t.Fatalf("drafts = %+v, err = %v, want only failed group retained", remaining, err)
	}
}

func TestApplyAction_OfflineReviewKeepsDraftsWithoutQueueing(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider(), err: errors.New("network connection reset")}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	queue := store.NewActionQueue(orch.store.DB)
	orch.WithActionQueue(queue)
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 1, Body: "offline", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.SubmitReviewCommand, ReviewEvent: actions.ReviewEventComment})

	if result.Success || !result.IsAmbiguousRemoteSuccess() || !strings.Contains(result.Error, "1 draft(s) kept (a.go:1)") {
		t.Fatalf("result = %+v, want ambiguous result naming the kept draft", result)
	}
	remaining, err := drafts.List(context.Background(), "pr-r", "me")
	if err != nil || len(remaining) != 1 {
		t.Fatalf("drafts = %+v, err = %v, want offline draft retained", remaining, err)
	}
	pending, err := queue.GetPending(context.Background(), 1)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending = %+v, err = %v, want no queued review", pending, err)
	}
}

func TestApplyAction_CommentReviewWithNothingToSubmitFails(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, _ := newReviewTestOrchestrator(t, prov)
	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.SubmitReviewCommand, ReviewEvent: actions.ReviewEventComment})
	if result.Success || len(prov.submissions) != 0 {
		t.Fatalf("result = %+v, want failure without a provider call", result)
	}
}

func TestApplyAction_InvalidReviewPostsNoOlderGroups(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider()}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "old.go", Side: "RIGHT", Line: 2, Body: "outdated", CommitSHA: "old"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.SubmitReviewCommand, ReviewEvent: actions.ReviewEventRequestChanges})

	if result.Success || prov.calls != 0 {
		t.Fatalf("result = %+v, calls = %d, want rejection before any review posts", result, prov.calls)
	}
}

func TestApplyAction_SubmitReviewIsAttemptedOnce(t *testing.T) {
	// A retryable failure still must not resend: GitHub may have accepted the batch.
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider(), err: errors.New("POST .../reviews: 429 Too Many Requests")}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 1, Body: "one", CommitSHA: "head"})

	orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.SubmitReviewCommand, ReviewEvent: actions.ReviewEventComment})

	if prov.calls != 1 {
		t.Fatalf("SubmitReview calls = %d, want 1", prov.calls)
	}
}

func TestApplyAction_AmbiguousReviewResultIsNotReportedAsUnsubmitted(t *testing.T) {
	prov := &reviewRecorder{MockProvider: mock.NewMockProvider(), err: errors.New("POST .../reviews: Client.Timeout exceeded")}
	orch, drafts := newReviewTestOrchestrator(t, prov)
	mustDraft(t, drafts, store.ReviewDraft{Path: "a.go", Side: "RIGHT", Line: 1, Body: "one", CommitSHA: "head"})

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{PRID: "pr-r", Action: actions.SubmitReviewCommand, ReviewEvent: actions.ReviewEventComment})

	if !result.IsAmbiguousRemoteSuccess() || strings.Contains(result.Error, "not submitted") {
		t.Fatalf("result = %+v, want the ambiguous result unchanged", result)
	}
}
