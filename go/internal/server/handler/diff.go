package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/samuelasvanyi/bistro/internal/store"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

// diffReader is the subset of store.DiffStore required by the REQUEST_DIFF handler.
type diffReader interface {
	GetUnifiedDiff(prID string) (string, error)
}

// NewRequestDiffHandler returns a ws.MessageHandler that reads diffs from the
// database and returns them as a DIFF_READY message over WebSocket.
func NewRequestDiffHandler(ds diffReader, log *slog.Logger) ws.MessageHandler {
	return func(msg ws.InboundMessage) (interface{}, error) {
		var data struct {
			PRID string `json:"pr_id"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid REQUEST_DIFF payload: %w", err)
		}
		if data.PRID == "" {
			return nil, fmt.Errorf("REQUEST_DIFF: pr_id is required")
		}

		unified, err := ds.GetUnifiedDiff(data.PRID)
		if err != nil {
			if log != nil {
				log.Warn("failed to get diff", "pr_id", data.PRID, "err", err)
			}
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

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "DIFF_READY",
			"data": map[string]interface{}{
				"pr_id": data.PRID,
				"diff":  unified,
			},
		}, nil
	}
}

var _ diffReader = (*store.DiffStore)(nil)
