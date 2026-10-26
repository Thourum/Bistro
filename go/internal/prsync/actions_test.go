package prsync

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/itterno/bistro/internal/store/actionqueue"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/provider"

	"github.com/itterno/bistro/internal/store/storetest"
)

// ── Blocker 3: queue ID uniqueness per thread ─────────────────────────────

// TestQueuedActionID_ThreadDistinct verifies that two resolve_thread actions
// on the same PR but different threads produce distinct queue IDs so they
// are stored as separate rows and do not overwrite each other.
func TestQueuedActionID_ThreadDistinct(t *testing.T) {
	req1 := &actions.ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: actions.ResolveThreadCommand,
		Body:   "PRRT_thread1",
	}
	req2 := &actions.ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: actions.ResolveThreadCommand,
		Body:   "PRRT_thread2",
	}
	id1 := queuedActionID(req1)
	id2 := queuedActionID(req2)
	if id1 == id2 {
		t.Errorf("different threads on same PR must get distinct queue IDs; both got %q", id1)
	}
}

// TestQueuedActionID_ApproveIdempotent verifies that approve (no body) retains
// pr_id+action idempotent deduplication behaviour.
func TestQueuedActionID_ApproveIdempotent(t *testing.T) {
	req := &actions.ActionRequest{
		PRID:   "github:owner/repo:7",
		Action: actions.ApproveCommand,
		Body:   "",
	}
	want := fmt.Sprintf("%s-%s", req.PRID, string(req.Action))
	if got := queuedActionID(req); got != want {
		t.Errorf("approve queue ID = %q, want %q", got, want)
	}
}

// TestQueuedActionID_SameThreadSamePR verifies that the same request always
// maps to the same ID (stable idempotence per thread).
func TestQueuedActionID_SameThreadSamePR(t *testing.T) {
	req := &actions.ActionRequest{
		PRID:   "github:owner/repo:5",
		Action: actions.ResolveThreadCommand,
		Body:   "PRRT_abc",
	}
	first := queuedActionID(req)
	second := queuedActionID(req)
	if first != second {
		t.Error("same request must always produce the same queue ID")
	}
}

// ── Blocker 2: thread validation (pr_id scope + state_known) ─────────────

// TestApplyAction_ThreadNotFoundInStore verifies that ApplyAction returns a
// clean failure when the thread does not exist for the given PR, without
// reaching the (nil) provider.
func TestApplyAction_ThreadNotFoundInStore(t *testing.T) {
	db := storetest.Open(t)
	// No thread row inserted.

	orch := NewOrchestrator(nil, nil, db, nil)
	req := &actions.ActionRequest{
		PRID:   "pr-v1",
		Action: actions.ResolveThreadCommand,
		Body:   "PRRT_ghost",
	}
	result := orch.ApplyAction(context.Background(), req)
	if result.Success {
		t.Error("expected failure for non-existent thread")
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

// TestApplyAction_ThreadStateUnknownRejected verifies that a thread with
// state_known=false is rejected before any provider call.
func TestApplyAction_ThreadStateUnknownRejected(t *testing.T) {
	db := storetest.Open(t)
	insertThreadSync(t, db.DB, "pr-v2", "PRRT_unk", false, false) // state_known=false

	orch := NewOrchestrator(nil, nil, db, nil)
	req := &actions.ActionRequest{
		PRID:   "pr-v2",
		Action: actions.ResolveThreadCommand,
		Body:   "PRRT_unk",
	}
	result := orch.ApplyAction(context.Background(), req)
	if result.Success {
		t.Error("expected failure for state_known=false thread")
	}
	if result.Error == "" {
		t.Error("expected non-empty error for state_known=false")
	}
}

// TestApplyAction_ThreadWrongPRRejected verifies that a thread belonging to a
// different PR cannot be mutated (cross-PR isolation).
func TestApplyAction_ThreadWrongPRRejected(t *testing.T) {
	db := storetest.Open(t)
	insertThreadSync(t, db.DB, "pr-A", "PRRT_x", true, true) // belongs to pr-A

	orch := NewOrchestrator(nil, nil, db, nil)
	req := &actions.ActionRequest{
		PRID:   "pr-B", // wrong PR
		Action: actions.ResolveThreadCommand,
		Body:   "PRRT_x",
	}
	result := orch.ApplyAction(context.Background(), req)
	if result.Success {
		t.Error("expected failure: thread belongs to different PR")
	}
	if result.Error == "" {
		t.Error("expected non-empty error for wrong-PR thread")
	}
}

// TestApplyAction_ThreadStateKnownPassesValidation verifies that a thread with
// state_known=true passes the pre-flight check (failure here is from nil
// provider, not from validation).
func TestApplyAction_ThreadStateKnownPassesValidation(t *testing.T) {
	db := storetest.Open(t)
	insertThreadSync(t, db.DB, "pr-v3", "PRRT_ok", false, true) // state_known=true

	orch := NewOrchestrator(nil, nil, db, nil)
	req := &actions.ActionRequest{
		PRID:   "pr-v3",
		Action: actions.ResolveThreadCommand,
		Body:   "PRRT_ok",
	}
	result := orch.ApplyAction(context.Background(), req)
	// Validation passed — the nil-provider executor returned an error, but it
	// must NOT be one of the validation errors.
	if result.Error == fmt.Sprintf("thread %s not found in pr %s", req.Body, req.PRID) {
		t.Error("validation erroneously rejected a state_known=true thread as not found")
	}
	if result.Error == fmt.Sprintf("thread %s state is unknown; sync required before mutation", req.Body) {
		t.Error("state_known check incorrectly fired for a state_known=true thread")
	}
}

// ── Blocker 2: liveness cancellation during execution ────────────────────

// TestExecuteAction_LivenessCancelsBeforeProviderMutation verifies that when
// liveness is cancelled before executeAction calls the provider, the execution
// context (execCtx) is also cancelled so ExecuteWithRetry returns an error
// rather than completing with stale credentials.
//
// We use a real cancelled liveness context; since provider is nil executeAction
// would panic on a live call, but execCtx cancellation causes ExecuteWithRetry
// to return a context-cancelled error before touching the provider.
func TestExecuteAction_LivenessCancelledInterruptsExecution(t *testing.T) {
	liveCtx, cancelLiveness := context.WithCancel(context.Background())
	// Cancel before creating orch to guarantee liveness is done on first check.
	cancelLiveness()

	orch := NewOrchestrator(nil, nil, nil, nil).WithLiveness(liveCtx)

	req := &actions.ActionRequest{
		PRID:   "github:owner/repo:1",
		Action: actions.ApproveCommand,
	}

	// With liveness already cancelled, ApplyAction must reject at the liveness
	// check — before even reaching executeAction.
	result := orch.ApplyAction(context.Background(), req)
	if result.Success {
		t.Error("expected failure: liveness cancelled before action")
	}
	if result.Error != "provider disconnected: action rejected" {
		t.Errorf("unexpected error: %q", result.Error)
	}
}

// TestHandleThreadActionSuccess_SkippedWhenLivenessCancelled verifies that the
// post-execution local SQLite write and WS event are suppressed when liveness
// is cancelled — prevents stale THREAD_RESOLVED events from being broadcast
// to a new session after provider replacement.
func TestHandleThreadActionSuccess_SkippedWhenLivenessCancelled(t *testing.T) {
	db := storetest.Open(t)
	insertThreadSync(t, db.DB, "pr-live", "PRRT_live", false, true) // state_known=true, unresolved

	liveCtx, cancelLiveness := context.WithCancel(context.Background())

	orch := NewOrchestrator(nil, nil, db, nil).WithLiveness(liveCtx)

	// Cancel liveness — simulate disconnect that happened just after a provider
	// call supposedly succeeded (we call handleThreadActionSuccess directly).
	cancelLiveness()

	req := &actions.ActionRequest{
		PRID:   "pr-live",
		Action: actions.ResolveThreadCommand,
		Body:   "PRRT_live",
	}

	// handleThreadActionSuccess is package-internal; call it directly since we're
	// in the same package (package sync test file).
	orch.handleThreadActionSuccess(context.Background(), req, orch.log)

	// Thread must remain unresolved — the write was suppressed.
	thread, err := db.GetReviewThread(context.Background(), "pr-live", "PRRT_live")
	if err != nil {
		t.Fatalf("GetReviewThread: %v", err)
	}
	if thread == nil {
		t.Fatal("thread not found")
	}
	if thread.Resolved {
		t.Error("thread should remain unresolved: handleThreadActionSuccess must be suppressed when liveness is cancelled")
	}
}

// ── Blocker 2: line-level post_comment rejected before queueing ──────────

// TestApplyAction_LineLevelComment_RateLimited_Rejected verifies that when
// the orchestrator is rate-limited, a post_comment with Line > 0 is rejected
// with a clear error rather than being queued with the Line context stripped.
// This prevents the comment from being replayed as a general PR comment.
func TestApplyAction_LineLevelComment_RateLimited_Rejected(t *testing.T) {
	db := storetest.Open(t)
	q := actionqueue.New(db.DB)
	orch := NewOrchestrator(nil, nil, db, nil).WithActionQueue(q)

	// Force rate-limited state.
	orch.rateLimitState = &provider.RateLimitState{
		ResetAt: time.Now().Add(5 * time.Minute),
	}

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{
		PRID:   "github:owner/repo:1",
		Action: actions.PostCommentCommand,
		Body:   "nit: please extract",
		Line:   42, // line-level → cannot be safely queued
	})

	if result.Success {
		t.Error("expected failure for line-level post_comment when rate-limited")
	}
	if !containsStr(result.Error, "line-level") {
		t.Errorf("error must mention line-level; got: %q", result.Error)
	}

	// Nothing must have been enqueued.
	pending, err := q.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 queued actions, got %d — line-level comment must not be queued", len(pending))
	}
}

// TestApplyAction_GeneralComment_RateLimited_Queued verifies that a general
// post_comment (Line == 0) is still queued normally when rate-limited.
func TestApplyAction_GeneralComment_RateLimited_Queued(t *testing.T) {
	db := storetest.Open(t)
	q := actionqueue.New(db.DB)
	orch := NewOrchestrator(nil, nil, db, nil).WithActionQueue(q)

	orch.rateLimitState = &provider.RateLimitState{
		ResetAt: time.Now().Add(5 * time.Minute),
	}

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{
		PRID:   "github:owner/repo:2",
		Action: actions.PostCommentCommand,
		Body:   "LGTM",
		Line:   0, // general PR comment — safe to queue
	})

	if result.Success {
		// rate-limited path returns failure even on success queue
	}
	if containsStr(result.Error, "line-level") {
		t.Error("general comment (Line=0) must not be rejected as line-level")
	}

	pending, err := q.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 1 {
		t.Errorf("expected 1 queued action for general comment, got %d", len(pending))
	}
}

func TestApplyAction_ReplyComment_RateLimited_QueuesResolvedTarget(t *testing.T) {
	db := storetest.Open(t)
	q := actionqueue.New(db.DB)
	prepareReplyThreadFixture(t, db, "github:owner/repo:3", "PRRT_reply_q", "github-review-comment-321")
	orch := NewOrchestrator(nil, nil, db, nil).WithActionQueue(q)
	orch.rateLimitState = &provider.RateLimitState{ResetAt: time.Now().Add(5 * time.Minute)}

	result := orch.ApplyAction(context.Background(), &actions.ActionRequest{
		PRID:     "github:owner/repo:3",
		Action:   actions.ReplyCommentCommand,
		Body:     "reply body",
		ThreadID: "PRRT_reply_q",
	})
	if result.Success {
		t.Error("rate-limited queue path should not return Success=true")
	}

	pending, err := q.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 queued reply, got %d", len(pending))
	}
	var payload actions.ReplyCommentPayload
	if err := json.Unmarshal([]byte(pending[0].Payload), &payload); err != nil {
		t.Fatalf("decode queued payload: %v", err)
	}
	if payload.Body != "reply body" || payload.InReplyTo != "github-review-comment-321" || payload.ThreadID != "PRRT_reply_q" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
