package prsync

import (
	"context"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/provider/mock"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
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

func TestDiscoverAndSyncInboxAdvancesCursorAfterWrite(t *testing.T) {
	db := storetest.Open(t)
	now := time.Date(2026, 6, 29, 22, 0, 0, 0, time.UTC)
	mockProv := mock.NewMockProvider().WithInboxDiscovery(&provider.InboxDiscoveryResult{
		PRs: []*provider.PR{{
			ID:         "github:org/repo:7",
			Provider:   "github",
			Owner:      "org",
			Repo:       "repo",
			Number:     7,
			Title:      "Inbox PR",
			Author:     "alice",
			State:      "open",
			BaseBranch: "main",
			HeadBranch: "feature",
			BaseSHA:    "base",
			HeadSHA:    "head",
			CreatedAt:  now.Add(-time.Hour),
			UpdatedAt:  now,
		}},
		HighWater: now,
		ETag:      "etag-2",
	}, nil)

	orch := NewOrchestrator(nil, mockProv, db, nil)
	result, err := orch.DiscoverAndSyncInbox(context.Background())
	if err != nil {
		t.Fatalf("DiscoverAndSyncInbox: %v", err)
	}
	if result.PRCount != 1 || result.UpdatedPRs != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	cur, err := db.GetSyncCursor(context.Background(), "github:inbox:mock-user:search:involves")
	if err != nil {
		t.Fatalf("GetSyncCursor: %v", err)
	}
	if cur == nil || cur.CursorValue != store.FormatSyncCursorTime(now) || cur.ETag != "etag-2" {
		t.Fatalf("unexpected cursor: %+v", cur)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM prs WHERE id = 'github:org/repo:7'`).Scan(&count); err != nil {
		t.Fatalf("count PR: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected hydrated PR to be written, got %d", count)
	}
}
