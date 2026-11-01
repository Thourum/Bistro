package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const maxPRsReadRequestSize = 500

type ReadState struct {
	DB          readStateDB
	CurrentUser currentUserReader
	Events      Emitter
	Panics      PanicReporter
	Log         *slog.Logger
}
type MarkReadRequest struct {
	PRIDs []string `json:"pr_ids"`
}
type MarkReadResult struct {
	Marked int `json:"marked"`
}

// MarkRead atomically records read timestamps and emits one change event.
func (s *ReadState) MarkRead(ctx context.Context, req MarkReadRequest) (MarkReadResult, error) {
	return guard(s.Panics, "ReadState.MarkRead", func() (MarkReadResult, error) {
		normalized := NormalizeIDs(req.PRIDs)
		if len(normalized) == 0 || len(normalized) > maxPRsReadRequestSize {
			return MarkReadResult{}, Errorf(CodeInvalid, "pr_ids must contain between 1 and %d IDs", maxPRsReadRequestSize)
		}
		prIDs, err := uniquePRIDs(normalized)
		if err != nil {
			return MarkReadResult{}, err
		}
		login, ok, err := s.CurrentUser.Get(ctx, githubProvider)
		if err != nil {
			return MarkReadResult{}, err
		}
		if !ok || login == "" {
			return MarkReadResult{}, Errorf(CodeUnauthenticated, "no authenticated user")
		}
		if err := markPRsRead(ctx, s.DB, prIDs, login, time.Now().UTC()); err != nil {
			return MarkReadResult{}, err
		}
		s.Events.Emit(EventPRsReadChanged, PRsReadChanged{PRIDs: prIDs})
		return MarkReadResult{Marked: len(prIDs)}, nil
	})
}
func uniquePRIDs(prIDs []string) ([]string, error) {
	unique := make([]string, 0, len(prIDs))
	seen := make(map[string]struct{}, len(prIDs))
	for _, prID := range prIDs {
		if prID == "" {
			return nil, Errorf(CodeInvalid, "pr_ids must not contain empty IDs")
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
