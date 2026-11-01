// go/internal/actions/integration_test.go
package actions

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
)

func TestFullActionFlow(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := events.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	requests := []*ActionRequest{
		{
			PRID:   "github:owner/repo:42",
			Action: ApproveCommand,
			Body:   "Approved!",
		},
		{
			PRID:   "github:owner/repo:42",
			Action: PostCommentCommand,
			Body:   "Great work!",
			Line:   10,
		},
	}

	results := executor.ExecuteMultiple(context.Background(), requests)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, result := range results {
		if !result.Success {
			t.Errorf("result %d failed: %s", i, result.Error)
		}
	}
}

func TestActionEventStream(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := events.NewEventEmitter()
	eventCh := emitter.Subscribe()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	go func() {
		executor.Execute(context.Background(), &ActionRequest{ //nolint:errcheck
			PRID:   "github:owner/repo:42",
			Action: ApproveCommand,
		})
	}()

	// Expect action_complete event
	select {
	case event := <-eventCh:
		if event.Type != "action_complete" {
			t.Errorf("expected action_complete, got %s", event.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestActionRetryOnTemporaryFailure(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := events.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.ExecuteWithRetry(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
	}, 3)

	if err != nil {
		t.Fatalf("ExecuteWithRetry error = %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Error)
	}
}

func TestActionContextCancellation(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := events.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := executor.ExecuteWithRetry(ctx, &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
	}, 3)

	if err != nil {
		t.Fatalf("ExecuteWithRetry error = %v", err)
	}
	if result == nil {
		t.Error("expected result even with cancelled context")
	}
}
