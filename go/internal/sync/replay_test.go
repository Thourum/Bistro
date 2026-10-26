package sync

import (
	"context"
	"encoding/json"
	stdsync "sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
)

// ── Blocker 3: replay validates thread preflight ──────────────────────────

// TestReplayQueuedActions_SkipsStateUnknownThread verifies that
// ReplayQueuedActions marks a queued thread action as permanently failed (not
// executed) when the thread's state_known=false, preventing stale mutations
// from reaching the provider.
func TestReplayQueuedActions_SkipsStateUnknownThread(t *testing.T) {
	db := storetest.Open(t)
	insertThreadSync(t, db.DB, "pr-replay", "PRRT_stale", false, false) // state_known=false

	q := store.NewActionQueue(db.DB)
	// Insert a queued resolve_thread action targeting the state_unknown thread.
	err := q.Enqueue(context.Background(), &store.QueuedAction{
		ID:         "pr-replay-resolve_thread-PRRT_stale",
		ActionType: string(actions.ResolveThreadCommand),
		PRID:       "pr-replay",
		Payload:    "PRRT_stale",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	orch := NewOrchestrator(nil, nil, db, nil).
		WithActionQueue(q)

	if err := orch.ReplayQueuedActions(context.Background()); err != nil {
		t.Fatalf("ReplayQueuedActions: %v", err)
	}

	// The action should be permanently failed — not pending.
	pending, err := q.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending actions after replay of state_unknown thread, got %d", len(pending))
	}
}

// TestReplayQueuedActions_SkipsWrongPRThread verifies that a queued thread
// action for a thread belonging to a different PR is permanently failed.
func TestReplayQueuedActions_SkipsWrongPRThread(t *testing.T) {
	db := storetest.Open(t)
	insertThreadSync(t, db.DB, "pr-correct", "PRRT_x", false, true) // belongs to pr-correct

	q := store.NewActionQueue(db.DB)
	// Queue action with wrong pr_id.
	err := q.Enqueue(context.Background(), &store.QueuedAction{
		ID:         "pr-wrong-resolve_thread-PRRT_x",
		ActionType: string(actions.ResolveThreadCommand),
		PRID:       "pr-wrong", // wrong PR
		Payload:    "PRRT_x",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	orch := NewOrchestrator(nil, nil, db, nil).
		WithActionQueue(q)

	if err := orch.ReplayQueuedActions(context.Background()); err != nil {
		t.Fatalf("ReplayQueuedActions: %v", err)
	}

	pending, err := q.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending actions after replay of wrong-PR thread, got %d", len(pending))
	}
}

func TestReplayQueuedActions_ProcessingPostCommentReconcilesRemoteSuccess(t *testing.T) {
	db := storetest.Open(t)
	q := store.NewActionQueue(db.DB)
	ctx := context.Background()

	const actionID = "pr-comment-crash-reconcile"
	const prID = "github:owner/repo:11"
	const body = "already posted while Bistro crashed"
	if err := q.Enqueue(ctx, &store.QueuedAction{
		ID:         actionID,
		ActionType: string(actions.PostCommentCommand),
		PRID:       prID,
		Payload:    body,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	queued, err := q.GetByID(ctx, actionID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if err := q.MarkProcessing(ctx, actionID); err != nil {
		t.Fatalf("MarkProcessing: %v", err)
	}

	prov := mock.NewMockProvider().WithComments([]*provider.Comment{{
		PRID:      prID,
		Author:    "mock-user",
		Body:      body,
		CreatedAt: queued.CreatedAt.Add(time.Second),
	}})
	orch := NewOrchestrator(nil, prov, db, nil).WithActionQueue(q)

	if err := orch.ReplayQueuedActions(ctx); err != nil {
		t.Fatalf("ReplayQueuedActions: %v", err)
	}
	got, err := q.GetByID(ctx, actionID)
	if err != nil {
		t.Fatalf("GetByID after replay: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %q, want completed", got.Status)
	}
	if len(prov.PostCommentCalls) != 0 {
		t.Fatalf("PostComment was called %d times; remote reconciliation should avoid duplicate mutation", len(prov.PostCommentCalls))
	}
}

func TestReplayQueuedActions_ConcurrentOrchestratorsDoNotDuplicatePostComment(t *testing.T) {
	db := storetest.Open(t)
	q := store.NewActionQueue(db.DB)
	ctx := context.Background()

	const actionID = "pr-comment-concurrent-replay"
	if err := q.Enqueue(ctx, &store.QueuedAction{
		ID:         actionID,
		ActionType: string(actions.PostCommentCommand),
		PRID:       "github:owner/repo:12",
		Payload:    "only once",
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	prov := &blockingPostCommentProvider{
		MockProvider: mock.NewMockProvider(),
		entered:      make(chan struct{}),
		release:      make(chan struct{}),
	}
	orch1 := NewOrchestrator(nil, prov, db, nil).WithActionQueue(q)
	orch2 := NewOrchestrator(nil, prov, db, nil).WithActionQueue(q)

	firstErr := make(chan error, 1)
	go func() { firstErr <- orch1.ReplayQueuedActions(ctx) }()

	select {
	case <-prov.entered:
	case <-time.After(time.Second):
		t.Fatal("first replay did not reach provider")
	}

	secondErr := make(chan error, 1)
	go func() { secondErr <- orch2.ReplayQueuedActions(ctx) }()

	time.Sleep(25 * time.Millisecond)
	close(prov.release)

	if err := <-firstErr; err != nil {
		t.Fatalf("first ReplayQueuedActions: %v", err)
	}
	if err := <-secondErr; err != nil {
		t.Fatalf("second ReplayQueuedActions: %v", err)
	}
	if got := prov.calls.Load(); got != 1 {
		t.Fatalf("PostComment calls = %d, want 1", got)
	}
	queued, err := q.GetByID(ctx, actionID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if queued.Status != "completed" {
		t.Fatalf("status = %q, want completed", queued.Status)
	}
}

func TestReplayTimestampMatchesAllowsLocalClockSkew(t *testing.T) {
	queuedAt := time.Date(2026, 6, 29, 23, 0, 0, 0, time.UTC)
	remoteCreatedAt := queuedAt.Add(-5 * time.Minute)
	if !replayTimestampMatches(remoteCreatedAt, queuedAt) {
		t.Fatal("remote success within clock-skew tolerance should reconcile")
	}
}

type blockingPostCommentProvider struct {
	*mock.MockProvider
	entered chan struct{}
	release chan struct{}
	once    stdsync.Once
	calls   atomic.Int32
}

func (p *blockingPostCommentProvider) PostComment(_ context.Context, _ string, _ int, _ string) error {
	p.calls.Add(1)
	p.once.Do(func() { close(p.entered) })
	<-p.release
	return nil
}

func TestReplayQueuedActions_ReplyComment_CallsProvider(t *testing.T) {
	db := storetest.Open(t)
	q := store.NewActionQueue(db.DB)
	prov := mock.NewMockProvider()
	orch := NewOrchestrator(nil, prov, db, nil).WithActionQueue(q)
	payload, _ := json.Marshal(actions.ReplyCommentPayload{Body: "reply body", InReplyTo: "github-review-comment-654"})
	if err := q.Enqueue(context.Background(), &store.QueuedAction{
		ID: "reply-queued", ActionType: string(actions.ReplyCommentCommand), PRID: "github:owner/repo:4", Payload: string(payload),
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := orch.ReplayQueuedActions(context.Background()); err != nil {
		t.Fatalf("ReplayQueuedActions: %v", err)
	}
	if len(prov.ReplyCalls) != 1 {
		t.Fatalf("expected one reply call, got %d", len(prov.ReplyCalls))
	}
	if prov.ReplyCalls[0].InReplyTo != "github-review-comment-654" || prov.ReplyCalls[0].Body != "reply body" {
		t.Fatalf("unexpected reply call: %+v", prov.ReplyCalls[0])
	}
}

func TestReplayQueuedActions_ReplyComment_InvalidPayloadTerminal(t *testing.T) {
	db := storetest.Open(t)
	q := store.NewActionQueue(db.DB)
	orch := NewOrchestrator(nil, mock.NewMockProvider(), db, nil).WithActionQueue(q)
	if err := q.Enqueue(context.Background(), &store.QueuedAction{
		ID: "reply-bad-json", ActionType: string(actions.ReplyCommentCommand), PRID: "github:owner/repo:5", Payload: "{bad-json",
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if err := orch.ReplayQueuedActions(context.Background()); err != nil {
		t.Fatalf("ReplayQueuedActions: %v", err)
	}
	pending, err := q.GetPending(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("invalid reply payload must be terminal, still pending: %+v", pending)
	}
}
