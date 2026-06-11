package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/samuelasvanyi/bistro/internal/logging"
	"github.com/samuelasvanyi/bistro/internal/store"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

// diffReader is the subset of store.DiffStore required by the REQUEST_DIFF handler.
type diffReader interface {
	GetUnifiedDiff(prID string) (string, error)
	GetDiffs(prID string) ([]store.DiffFile, error)
}

// diffFileStat is the per-file summary returned alongside the unified diff
// so the PR detail header can show file counts and +/- totals.
type diffFileStat struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// NewRequestDiffHandler returns a ws.MessageHandler that reads diffs from the
// database and returns them as a DIFF_READY message over WebSocket.
func NewRequestDiffHandler(ds diffReader, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var data struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid REQUEST_DIFF payload: %w", err)
		}
		if data.PRID == "" {
			return nil, fmt.Errorf("REQUEST_DIFF: pr_id is required")
		}

		logger.Debug("diff requested", "pr_id", data.PRID)

		unified, err := ds.GetUnifiedDiff(data.PRID)
		if err != nil {
			logger.Warn("failed to get diff", "pr_id", data.PRID, "err", err)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "DIFF_READY",
				"data": map[string]interface{}{
					"pr_id": data.PRID,
					"diff":  "",
					"error": err.Error(),
				},
			}, nil
		}

		files := []diffFileStat{}
		var additions, deletions int
		if diffFiles, filesErr := ds.GetDiffs(data.PRID); filesErr != nil {
			logger.Warn("failed to get diff file stats", "pr_id", data.PRID, "err", filesErr)
		} else {
			for _, f := range diffFiles {
				files = append(files, diffFileStat{
					Path:      f.FilePath,
					Additions: f.Additions,
					Deletions: f.Deletions,
				})
				additions += f.Additions
				deletions += f.Deletions
			}
		}

		logger.Debug("diff ready",
			"pr_id", data.PRID,
			"bytes", len(unified),
			"files", len(files),
			"additions", additions,
			"deletions", deletions,
		)

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "DIFF_READY",
			"data": map[string]interface{}{
				"pr_id":     data.PRID,
				"diff":      unified,
				"files":     files,
				"additions": additions,
				"deletions": deletions,
			},
		}, nil
	}
}

var _ diffReader = (*store.DiffStore)(nil)
