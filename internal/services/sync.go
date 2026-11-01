package services

import (
	"context"
	"log/slog"
)

type ForceSyncRequest struct {
	RepoID string `json:"repo_id"`
}
type SyncTriggered struct {
	RepoID string `json:"repo_id"`
	Queued bool   `json:"queued"`
}

// Sync queues manual sync requests without stalling when the loop is busy.
type Sync struct {
	Trigger       chan<- string
	CurrentStatus func() SyncStatus
	Panics        PanicReporter
	Log           *slog.Logger
}

// Force queues one repo, or all watched repos when repo_id is empty.
func (s *Sync) Force(ctx context.Context, req ForceSyncRequest) (SyncTriggered, error) {
	return guard(s.Panics, "Sync.Force", func() (SyncTriggered, error) {
		req.RepoID = NormalizeID(req.RepoID)
		if err := ctx.Err(); err != nil {
			return SyncTriggered{}, err
		}
		response := SyncTriggered{RepoID: req.RepoID}
		select {
		case s.Trigger <- req.RepoID:
			response.Queued = true
		default:
			serviceLogger(s.Log).Warn("sync trigger channel full, manual sync skipped")
		}
		return response, nil
	})
}

// Status reports the engine's current sync state for first paint.
func (s *Sync) Status(ctx context.Context) (SyncStatus, error) {
	return guard(s.Panics, "Sync.Status", func() (SyncStatus, error) {
		if err := ctx.Err(); err != nil {
			return SyncStatus{}, err
		}
		if s.CurrentStatus == nil {
			return SyncStatus{}, Errorf(CodeInternal, "sync status unavailable")
		}
		return s.CurrentStatus(), nil
	})
}
