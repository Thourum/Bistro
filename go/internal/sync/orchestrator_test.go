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

func TestSyncProgressTracking(t *testing.T) {
	progress := &SyncProgress{
		Stage:   "fetching_prs",
		Current: 5,
		Total:   50,
	}

	if progress.Stage != "fetching_prs" {
		t.Errorf("expected stage fetching_prs, got %s", progress.Stage)
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

func TestSyncWithProgressNilCallback(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil)
	req := &SyncRequest{
		Provider:   "github",
		Owner:      "org",
		Repo:       "repo",
		State:      "open",
		IncludePRs: false,
	}
	result, err := orch.SyncWithProgress(context.Background(), req, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
}

func TestSyncWithProgressCallback(t *testing.T) {
	orch := NewOrchestrator(nil, nil, nil, nil)
	req := &SyncRequest{
		Provider:   "github",
		Owner:      "org",
		Repo:       "repo",
		State:      "open",
		IncludePRs: false,
	}

	var stages []string
	result, err := orch.SyncWithProgress(context.Background(), req, func(p *SyncProgress) {
		stages = append(stages, p.Stage)
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil {
		t.Error("expected non-nil result")
	}
	if len(stages) == 0 {
		t.Error("expected at least one progress update")
	}
	if stages[0] != "starting" {
		t.Errorf("expected first stage 'starting', got %s", stages[0])
	}
}
