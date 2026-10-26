package prsync

import (
	"context"
	"strings"
	"testing"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store/actionqueue"
	"github.com/itterno/bistro/internal/store/storetest"
)

// Reconciliation succeeds with no matching remote records; only mutations fail.
type ambiguousReplayProvider struct {
	*mock.MockProvider
	mutationError error
	cancel        context.CancelFunc
	calls         int
}

func (p *ambiguousReplayProvider) Approve(context.Context, string) error {
	p.calls++
	if p.cancel != nil {
		p.cancel()
	}
	return p.mutationError
}

func (p *ambiguousReplayProvider) PostComment(context.Context, string, int, string) error {
	p.calls++
	if p.cancel != nil {
		p.cancel()
	}
	return p.mutationError
}

// Cancellation must happen inside the mutation: cancelling before replay only
// tests its database read failure, never the detached terminal write.
func TestReplayQueuedActions_AmbiguousRemoteSuccessIsTerminal(t *testing.T) {
	tests := []struct {
		name             string
		command          actions.ActionCommand
		payload          string
		mutationError    error
		cancelOnMutation bool
	}{
		{
			name:          "recoverable_provider_error",
			command:       actions.ApproveCommand,
			mutationError: &provider.ProviderError{Message: "503 service unavailable"},
		},
		{
			name:             "cancelled_context_after_provider_success",
			command:          actions.ApproveCommand,
			cancelOnMutation: true,
		},
		{
			name:             "context_cancelled_provider_error",
			command:          actions.PostCommentCommand,
			payload:          "review nit",
			mutationError:    context.Canceled,
			cancelOnMutation: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := actionqueue.New(storetest.Open(t).DB)
			const actionID = "ambiguous-replay"
			if err := q.Enqueue(context.Background(), &actionqueue.Action{
				ID: actionID, ActionType: string(tt.command), PRID: "github:owner/repo:42", Payload: tt.payload,
			}); err != nil {
				t.Fatalf("Enqueue: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			prov := &ambiguousReplayProvider{MockProvider: mock.NewMockProvider(), mutationError: tt.mutationError}
			if tt.cancelOnMutation {
				prov.cancel = cancel
			}
			orch := NewOrchestrator(nil, prov, nil, nil).WithActionQueue(q)
			if err := orch.ReplayQueuedActions(ctx); err != nil {
				t.Fatalf("ReplayQueuedActions: %v", err)
			}
			assertAmbiguousReplayTerminal(t, q, actionID)
			if err := orch.ReplayQueuedActions(context.Background()); err != nil {
				t.Fatalf("second ReplayQueuedActions: %v", err)
			}
			if prov.calls != 1 {
				t.Errorf("mutations = %d, want 1; terminal actions must not replay", prov.calls)
			}
		})
	}
}

func assertAmbiguousReplayTerminal(t *testing.T, q *actionqueue.Queue, actionID string) {
	t.Helper()
	action, err := q.GetByID(context.Background(), actionID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if action.Status != "failed" || action.RetryCount != actionqueue.MaxRetryCount {
		t.Errorf("queue state = %s/%d, want failed/%d", action.Status, action.RetryCount, actionqueue.MaxRetryCount)
	}
	if !strings.HasPrefix(action.LastError, actions.ErrAmbiguousRemoteSuccess) {
		t.Errorf("last_error = %q, want ambiguous remote success sentinel", action.LastError)
	}
	pending, err := q.GetPending(context.Background(), replayBatchSize)
	if err != nil {
		t.Fatalf("GetPending: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("pending actions = %d, want 0; ambiguous mutations must not be retried", len(pending))
	}
}
