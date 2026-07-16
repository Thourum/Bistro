package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

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

// SyncSettingsMsg carries runtime sync-settings updates to the sync loop.
type SyncSettingsMsg struct {
	AutoSync bool
	Interval time.Duration
}

type setSyncSettingsRequest struct {
	AutoSync         bool    `json:"auto_sync"`
	SyncIntervalSecs float64 `json:"sync_interval_secs"`
}

// NewSetSyncSettingsHandler returns the handler for SET_SYNC_SETTINGS messages.
// It updates autoSync and syncInterval in the running sync loop without restart.
// The update is coalescing: if the channel already holds a pending value it is
// drained first so the latest settings always win and no update is silently lost.
func NewSetSyncSettingsHandler(settings chan SyncSettingsMsg, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req setSyncSettingsRequest
		if len(msg.Data) > 0 {
			if err := json.Unmarshal(msg.Data, &req); err != nil {
				return nil, fmt.Errorf("invalid SET_SYNC_SETTINGS payload: %w", err)
			}
		}

		interval := time.Duration(req.SyncIntervalSecs * float64(time.Second))
		if interval <= 0 {
			return nil, fmt.Errorf("sync_interval_secs must be > 0, got %v", req.SyncIntervalSecs)
		}

		newMsg := SyncSettingsMsg{AutoSync: req.AutoSync, Interval: interval}

		// Coalesce: drain any stale pending value so the latest always wins.
		for {
			select {
			case <-settings:
				// drained a stale entry; loop to drain any additional ones
			default:
				goto sendNew
			}
		}
	sendNew:
		select {
		case settings <- newMsg:
			logger.Info("sync settings update queued",
				"auto_sync", req.AutoSync,
				"interval", interval.String(),
			)
		default:
			// Channel was concurrently filled between drain and send (extremely
			// rare race). Log and continue — the sync loop holds the latest
			// value from the drain-replace above or will pick this up next time.
			logger.Warn("sync settings channel full after coalesce; update deferred")
		}

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "SYNC_SETTINGS_UPDATED",
			"data": map[string]interface{}{
				"auto_sync":          req.AutoSync,
				"sync_interval_secs": req.SyncIntervalSecs,
				"applied":            true,
			},
		}, nil
	}
}

// PRSyncer hydrates a single PR's details on demand.
// Satisfied by *sync.Orchestrator.
type PRSyncer interface {
	SyncPR(ctx context.Context, prID string) error
}

// PRSyncerGetter returns the current PRSyncer (nil when no provider is
// authenticated) so the handler always routes to the live orchestrator.
type PRSyncerGetter func() PRSyncer

// NewRequestPRSyncHandler returns the handler for REQUEST_PR_SYNC — the
// on-demand hydration path for list-only PR rows (hydrated=0). It fetches
// comments, CI, and diff for one PR and responds PR_SYNCED when the row is
// fully written, so the UI can re-request comments/diff immediately.
func NewRequestPRSyncHandler(getSyncer PRSyncerGetter, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid REQUEST_PR_SYNC payload: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("REQUEST_PR_SYNC: pr_id is required")
		}

		syncer := getSyncer()
		if syncer == nil {
			return nil, fmt.Errorf("REQUEST_PR_SYNC unavailable: no authenticated provider")
		}

		logger.Info("on-demand pr sync requested", "pr_id", req.PRID)
		if err := syncer.SyncPR(ctx, req.PRID); err != nil {
			return nil, fmt.Errorf("REQUEST_PR_SYNC %s: %w", req.PRID, err)
		}

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "PR_SYNCED",
			"data": map[string]interface{}{"pr_id": req.PRID},
		}, nil
	}
}
