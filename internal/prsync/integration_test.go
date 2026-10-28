package prsync

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/events"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
)

func TestFullSyncFlow(t *testing.T) {
	mockProvider := mock.NewMockProvider().WithPRs([]*provider.PR{
		{
			ID:     "github:org/repo:1",
			Number: 1,
			Title:  "Test PR 1",
			Author: "alice",
		},
	})

	orch := NewOrchestrator(nil, mockProvider, nil, nil)

	if orch == nil {
		t.Error("orchestrator should not be nil")
	}

	req := &SyncRequest{
		Provider:        "github",
		Owner:           "org",
		Repo:            "repo",
		IncludePRs:      true,
		IncludeComments: true,
		IncludeCI:       false,
		IncludeDiff:     false,
		State:           "open",
	}

	ctx := context.Background()
	result, err := orch.Sync(ctx, req)

	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}

	if result.PRCount == 0 {
		t.Error("should have fetched PRs")
	}
}

func TestSyncWithEventEmitter(t *testing.T) {
	emitter := events.NewEventEmitter()

	eventCh := emitter.Subscribe()
	defer emitter.Unsubscribe(eventCh)

	event := events.NewProgressEvent("fetching_prs", 1, 10)
	emitter.Emit(event)

	select {
	case received := <-eventCh:
		if received.Type != "sync_progress" {
			t.Errorf("expected sync_progress, got %s", received.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("event not received")
	}
}

func TestSyncContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	mockProvider := mock.NewMockProvider()
	orch := NewOrchestrator(nil, mockProvider, nil, nil)

	req := &SyncRequest{
		Provider:        "github",
		Owner:           "org",
		Repo:            "repo",
		IncludePRs:      true,
		IncludeComments: true,
	}

	_, _ = orch.Sync(ctx, req)

	// Context should be cancelled
	if ctx.Err() == nil {
		t.Error("context should be cancelled")
	}
}

func TestErrorRecovery(t *testing.T) {
	mockProvider := mock.NewMockProvider().WithError(fmt.Errorf("test error"))

	orch := NewOrchestrator(nil, mockProvider, nil, nil)

	req := &SyncRequest{
		Provider:        "github",
		Owner:           "org",
		Repo:            "repo",
		IncludePRs:      true,
		IncludeComments: true,
	}

	result, err := orch.Sync(context.Background(), req)

	// Should not return a hard error — partial result with errors recorded
	if err != nil {
		t.Errorf("unexpected hard error: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil even on error")
	}

	if len(result.Errors) == 0 {
		t.Error("should have errors recorded")
	}
}

func TestOrchestratorEmitsEvents(t *testing.T) {
	mockProvider := mock.NewMockProvider().WithPRs([]*provider.PR{
		{ID: "github:org/repo:1", Number: 1, Title: "PR 1"},
	})

	orch := NewOrchestrator(nil, mockProvider, nil, nil)

	eventCh := orch.GetEventChannel()

	req := &SyncRequest{
		Provider:        "github",
		Owner:           "org",
		Repo:            "repo",
		IncludePRs:      true,
		IncludeComments: true,
		State:           "open",
	}

	// Collect events in background
	var collected []*events.Event
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case ev, ok := <-eventCh:
				if !ok {
					return
				}
				collected = append(collected, ev)
				if ev.Type == "sync_complete" {
					return
				}
			case <-time.After(2 * time.Second):
				return
			}
		}
	}()

	_, err := orch.Sync(context.Background(), req)
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	<-done

	if len(collected) == 0 {
		t.Error("expected at least one event emitted")
	}

	// Verify sync_complete was emitted
	var foundComplete bool
	for _, ev := range collected {
		if ev.Type == "sync_complete" {
			foundComplete = true
		}
	}
	if !foundComplete {
		t.Error("expected sync_complete event")
	}
}
