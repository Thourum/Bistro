package sync

import (
	"context"
	"testing"
)

func TestNewOrchestrator(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil)
	if orch == nil {
		t.Error("expected non-nil orchestrator")
	}
}

func TestSyncRequestValidation(t *testing.T) {
	req := &SyncRequest{
		Provider: "github",
		Owner:    "org",
		Repo:     "repo",
		State:    "open",
	}

	if req.Provider != "github" {
		t.Error("expected provider github")
	}
	if req.Owner == "" {
		t.Error("owner should not be empty")
	}
}

func TestSyncResultCreation(t *testing.T) {
	result := &SyncResult{
		PRCount:    42,
		NewPRs:     3,
		UpdatedPRs: 5,
		DiffCount:  8,
	}

	if result.PRCount != 42 {
		t.Errorf("expected 42 PRs, got %d", result.PRCount)
	}
}

func TestSyncNilRequest(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil)
	_, err := orch.Sync(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil request")
	}
}

func TestSyncMissingFields(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil)
	_, err := orch.Sync(context.Background(), &SyncRequest{})
	if err == nil {
		t.Error("expected error for empty request fields")
	}
}

