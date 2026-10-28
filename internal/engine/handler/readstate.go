package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/ws"
)

const maxPRsReadRequestSize = 500

type readStateDB interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

type broadcaster interface {
	Broadcast(msg interface{})
}

// pushMessage is a server-initiated {type, data} broadcast.
type pushMessage struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// NewMarkPRsReadHandler returns a WS handler for MARK_PRS_READ.
//
// Request: {"pr_ids": ["github:owner/repo:1", ...]}
// Response type: PRS_READ_ACK
// Response data: {"ok": true/false, "marked": N, "reason"?: "..."}
func NewMarkPRsReadHandler(db readStateDB, cu currentUserReader, bc broadcaster, log *slog.Logger) ws.MessageHandler {
	if log == nil {
		log = slog.Default()
	}

	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req struct {
			PRIDs []string `json:"pr_ids"`
		}
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("parse MARK_PRS_READ: %w", err)
		}
		if len(req.PRIDs) == 0 {
			return nil, fmt.Errorf("MARK_PRS_READ requires at least one pr_id")
		}
		if len(req.PRIDs) > maxPRsReadRequestSize {
			return nil, fmt.Errorf("MARK_PRS_READ accepts at most %d pr_ids", maxPRsReadRequestSize)
		}
		prIDs, err := uniquePRIDs(req.PRIDs)
		if err != nil {
			return nil, err
		}

		login, ok, err := cu.Get(ctx, githubProvider)
		if err != nil {
			logger.Error("resolve current user for MARK_PRS_READ", "error", err)
			return markPRsReadACK(msg.ID, false, 0, "unable to update read state"), nil
		}
		if !ok || login == "" {
			logger.Warn("MARK_PRS_READ: no current user, skipping", "pr_count", len(prIDs))
			return markPRsReadACK(msg.ID, false, 0, "no authenticated user"), nil
		}

		if err := markPRsRead(ctx, db, prIDs, login, time.Now().UTC()); err != nil {
			logger.Error("mark PRs read failed", "pr_count", len(prIDs), "error", err)
			return markPRsReadACK(msg.ID, false, 0, "unable to update read state"), nil
		}

		bc.Broadcast(pushMessage{
			Type: "PRS_READ_CHANGED",
			Data: map[string]interface{}{"pr_ids": prIDs},
		})
		logger.Debug("PRs marked read", "pr_count", len(prIDs), "login", login)

		return markPRsReadACK(msg.ID, true, len(prIDs), ""), nil
	}
}

func uniquePRIDs(prIDs []string) ([]string, error) {
	unique := make([]string, 0, len(prIDs))
	seen := make(map[string]struct{}, len(prIDs))
	for _, prID := range prIDs {
		if prID == "" {
			return nil, fmt.Errorf("MARK_PRS_READ requires non-empty pr_ids")
		}
		if _, exists := seen[prID]; exists {
			continue
		}
		seen[prID] = struct{}{}
		unique = append(unique, prID)
	}
	return unique, nil
}

func markPRsRead(ctx context.Context, db readStateDB, prIDs []string, login string, lastReadAt time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin read-state transaction: %w", err)
	}
	defer tx.Rollback()

	const upsertReadState = `INSERT INTO pr_read_state (pr_id, login, last_read_at)
		VALUES (?, ?, ?)
		ON CONFLICT(pr_id, login) DO UPDATE SET last_read_at = excluded.last_read_at`
	for _, prID := range prIDs {
		if _, err := tx.ExecContext(ctx, upsertReadState, prID, login, lastReadAt); err != nil {
			return fmt.Errorf("upsert read state for PR %q: %w", prID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit read-state transaction: %w", err)
	}
	return nil
}

func markPRsReadACK(requestID string, ok bool, marked int, reason string) map[string]interface{} {
	data := map[string]interface{}{
		"ok":     ok,
		"marked": marked,
	}
	if reason != "" {
		data["reason"] = reason
	}
	return map[string]interface{}{
		"id":   requestID,
		"type": "PRS_READ_ACK",
		"data": data,
	}
}
