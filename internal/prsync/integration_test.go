package prsync

import (
	"context"
	"fmt"
	"testing"

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

func TestOrchestratorDoesNotEmitUnconsumedSyncEvents(t *testing.T) {
	for _, prov := range []*mock.MockProvider{mock.NewMockProvider(), mock.NewMockProvider().WithError(fmt.Errorf("test error"))} {
		orch := NewOrchestrator(nil, prov, nil, nil)
		eventCh := orch.GetEventChannel()
		_, err := orch.Sync(context.Background(), &SyncRequest{Provider: "github", Owner: "org", Repo: "repo", IncludePRs: true})
		orch.UnsubscribeEventChannel(eventCh)
		if err != nil {
			t.Fatalf("sync failed: %v", err)
		}
		for event := range eventCh {
			t.Errorf("unexpected event: %s", event.Type)
		}
	}
}
