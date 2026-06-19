package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/ws"
)

type commentsRequest struct {
	PRID string `json:"pr_id"`
}

type commentRow struct {
	ID        string  `json:"id"`
	PRID      string  `json:"pr_id"`
	Author    string  `json:"author"`
	Body      string  `json:"body"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Line      *int    `json:"line,omitempty"`
	File      *string `json:"file,omitempty"`
}

// NewRequestCommentsHandler returns the handler for REQUEST_COMMENTS messages.
// It reads cached comments for a PR from SQLite and responds with a
// COMMENTS_READY envelope correlated by the request id.
func NewRequestCommentsHandler(db prLister, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req commentsRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid REQUEST_COMMENTS payload: %w", err)
		}
		if req.PRID == "" {
			return nil, fmt.Errorf("REQUEST_COMMENTS requires pr_id")
		}

		logger.Debug("comments requested", "pr_id", req.PRID)

		rows, err := db.Query(
			`SELECT id, pr_id, author, body, created_at, updated_at, line_number, file_path
			 FROM comments WHERE pr_id = ? ORDER BY created_at ASC`,
			req.PRID,
		)
		if err != nil {
			logger.Error("REQUEST_COMMENTS query failed", "pr_id", req.PRID, "err", err)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "COMMENTS_READY",
				"data": map[string]interface{}{
					"pr_id":    req.PRID,
					"comments": []commentRow{},
					"error":    err.Error(),
				},
			}, nil
		}
		defer rows.Close()

		comments := []commentRow{}
		for rows.Next() {
			var c commentRow
			if err := rows.Scan(
				&c.ID, &c.PRID, &c.Author, &c.Body,
				&c.CreatedAt, &c.UpdatedAt, &c.Line, &c.File,
			); err != nil {
				logger.Error("REQUEST_COMMENTS scan failed", "pr_id", req.PRID, "err", err)
				continue
			}
			comments = append(comments, c)
		}

		logger.Debug("comments ready", "pr_id", req.PRID, "count", len(comments))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "COMMENTS_READY",
			"data": map[string]interface{}{
				"pr_id":    req.PRID,
				"comments": comments,
			},
		}, nil
	}
}
