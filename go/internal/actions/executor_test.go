// go/internal/actions/executor_test.go
package actions

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/provider/mock"
	"github.com/samuelasvanyi/bistro/internal/sync"
)

func TestExecutorApprove(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
		Body:   "Approved!",
	})

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
}

func TestExecutorEventEmission(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := sync.NewEventEmitter()
	eventCh := emitter.Subscribe()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	go executor.Execute(context.Background(), &ActionRequest{ //nolint:errcheck
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
	})

	// Wait for event with timeout
	select {
	case event := <-eventCh:
		if event.Type != "action_complete" {
			t.Errorf("expected action_complete event, got %s", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestExecutorWithRetry(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.ExecuteWithRetry(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: ApproveCommand,
	}, 3)

	if err != nil {
		t.Fatalf("ExecuteWithRetry() error = %v", err)
	}
	if !result.Success {
		t.Errorf("expected success after retry, got: %s", result.Error)
	}
}

func TestExecutorErrorHandling(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithError(fmt.Errorf("mock error"))
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:99",
		Action: ApproveCommand,
	})

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Success {
		t.Error("expected failure")
	}
}

func TestExecutorInvalidAction(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider()
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	result, err := executor.Execute(context.Background(), &ActionRequest{
		PRID:   "github:owner/repo:42",
		Action: "invalid_action",
	})

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Success {
		t.Error("expected failure for invalid action")
	}
}

func TestExecutorExecuteMultiple(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	mockProv := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:owner/repo:42"},
	})
	emitter := sync.NewEventEmitter()

	executor := NewActionExecutor(log, mockProv, nil, emitter)

	requests := []*ActionRequest{
		{PRID: "github:owner/repo:42", Action: ApproveCommand},
		{PRID: "github:owner/repo:42", Action: PostCommentCommand, Body: "LGTM"},
	}

	results := executor.ExecuteMultiple(context.Background(), requests)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if !r.Success {
			t.Errorf("result[%d] expected success, got: %s", i, r.Error)
		}
	}
}
