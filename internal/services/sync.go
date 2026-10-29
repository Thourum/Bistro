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
	Trigger chan<- string
	Panics  PanicReporter
	Log     *slog.Logger
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
