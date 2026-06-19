package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/ws"
)

type forceSyncRequest struct {
	RepoID string `json:"repo_id"`
}

// NewForceSyncHandler returns the handler for FORCE_SYNC messages.
// An empty repo_id requests a sync of all watched repos. The trigger send is
// non-blocking: when a sync is already saturating the queue, the request is
// reported as not queued instead of stalling the dispatcher.
func NewForceSyncHandler(trigger chan<- string, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req forceSyncRequest
		if len(msg.Data) > 0 {
			if err := json.Unmarshal(msg.Data, &req); err != nil {
				return nil, fmt.Errorf("invalid FORCE_SYNC payload: %w", err)
			}
		}

		queued := false
		select {
		case trigger <- req.RepoID:
			queued = true
			logger.Info("manual sync triggered", "repo_id", req.RepoID)
		default:
			logger.Warn("sync trigger channel full, manual sync skipped", "repo_id", req.RepoID)
		}

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "SYNC_TRIGGERED",
			"data": map[string]interface{}{
				"repo_id": req.RepoID,
				"queued":  queued,
			},
		}, nil
	}
}
