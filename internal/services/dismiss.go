package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/store/currentuser"
	"github.com/itterno/bistro/internal/store/dismissal"
)

type DismissalScopeGetter func() string
type Dismiss struct {
	DB          *sql.DB
	Dismissals  *dismissal.Store
	CurrentUser *currentuser.Store
	GetApplier  ApplierGetter
	Scope       DismissalScopeGetter
	Panics      PanicReporter
	Log         *slog.Logger
}
type DismissRequest struct {
	PRID       string   `json:"pr_id"`
	CommentIDs []string `json:"comment_ids"`
}
type UndismissRequest struct {
	CommentIDs         []string `json:"comment_ids"`
	UnresolveThreadIDs []string `json:"unresolve_thread_ids"`
}
type DismissResult struct {
	PRID              string   `json:"pr_id"`
	BatchID           string   `json:"batch_id"`
	DismissedIDs      []string `json:"dismissed_ids"`
	ResolvedThreadIDs []string `json:"resolved_thread_ids"`
}
type UndismissResult struct {
	UndismissedIDs      []string `json:"undismissed_ids"`
	UnresolvedThreadIDs []string `json:"unresolved_thread_ids"`
}

// currentAccount resolves the account dismissals are namespaced under.
// Dismissals are per-account (spec 3.5); with no current_user row there is
// no account to namespace under, and a fallback row would be invisible to
// the serve-side join (which only matches current_user accounts) — so this
// errors instead of writing rows that vanish after login.
func (d Dismiss) currentAccount(ctx context.Context) (provider, login string, err error) {
	row := d.DB.QueryRowContext(ctx, `SELECT provider, login FROM current_user ORDER BY provider LIMIT 1`)
	if scanErr := row.Scan(&provider, &login); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			return "", "", Errorf(CodeUnauthenticated, "dismissals require a signed-in account")
		}
		return "", "", scanErr
	}
	if login == "" {
		return "", "", Errorf(CodeUnauthenticated, "dismissals require a signed-in account")
	}
	return provider, login, nil
}

// resolvableThreadIDs returns the distinct thread ids among the given
// comments whose stored state is known and unresolved — the only threads a
// provider write-back may resolve (spec: unknown/already-resolved threads
// silently stay local-only).
func (d Dismiss) resolvableThreadIDs(ctx context.Context, commentIDs []string) ([]string, error) {
	if len(commentIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(commentIDs)), ",")
	args := make([]interface{}, len(commentIDs))
	for i, id := range commentIDs {
		args[i] = id
	}
	rows, err := d.DB.QueryContext(ctx,
		`SELECT DISTINCT t.id
		 FROM comments c
		 JOIN review_threads t ON t.id = c.thread_id
		 WHERE c.id IN (`+placeholders+`) AND t.state_known = 1 AND t.resolved = 0
		 ORDER BY t.id`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("resolvable threads query: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("resolvable threads scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// submitThreadActions submits one thread action per id via the current
// applier. Failures ride the offline action queue (the applier's contract);
// a nil applier skips write-back entirely — the local rows always stand.
func (d Dismiss) submitThreadActions(ctx context.Context, log *slog.Logger, prID string, cmd actions.ActionCommand, threadIDs []string) {
	if len(threadIDs) == 0 || d.GetApplier == nil {
		return
	}
	applier := d.GetApplier()
	if applier == nil {
		log.Debug("no applier; provider write-back skipped", "action", cmd, "threads", len(threadIDs))
		return
	}
	for _, tid := range threadIDs {
		req := &actions.ActionRequest{
			PRID:     prID,
			Action:   cmd,
			Body:     tid,
			ThreadID: tid,
		}
		result := applier.ApplyAction(ctx, req)
		if result != nil && !result.Success {
			log.Warn("thread write-back not applied immediately (queued or failed)",
				"action", cmd, "thread_id", tid, "error", result.Error)
		}
	}
}

func newBatchID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "batch-fallback"
	}
	return hex.EncodeToString(b[:])
}

// Dismiss persists local dismissals before best-effort provider resolution.
func (s *Dismiss) Dismiss(ctx context.Context, req DismissRequest) (DismissResult, error) {
	return guard(s.Panics, "Dismiss.Dismiss", func() (DismissResult, error) {
		prID := NormalizeID(req.PRID)
		if prID == "" || len(req.CommentIDs) == 0 {
			return DismissResult{}, Errorf(CodeInvalid, "pr_id and comment_ids are required")
		}
		logger := serviceLogger(s.Log)
		provider, login, err := s.currentAccount(ctx)
		if err != nil {
			return DismissResult{}, err
		}
		if err := s.Dismissals.Dismiss(ctx, req.CommentIDs, provider, login); err != nil {
			return DismissResult{}, fmt.Errorf("dismiss failed: %w", err)
		}

		resolvedThreadIDs := []string{}
		scope := "local"
		if s.Scope != nil {
			scope = s.Scope()
		}
		if scope == "local+provider" {
			ids, err := s.resolvableThreadIDs(ctx, req.CommentIDs)
			if err != nil {
				// Local dismissal stands; write-back is best-effort.
				logger.Warn("resolvable-thread lookup failed; write-back skipped", "err", err)
			} else if len(ids) > 0 {
				s.submitThreadActions(ctx, logger, prID, actions.ResolveThreadCommand, ids)
				resolvedThreadIDs = ids
			}
		}

		return DismissResult{PRID: prID, BatchID: newBatchID(), DismissedIDs: req.CommentIDs, ResolvedThreadIDs: resolvedThreadIDs}, nil
	})
}

// Undismiss restores local comments and reopens only explicitly requested threads.
func (s *Dismiss) Undismiss(ctx context.Context, req UndismissRequest) (UndismissResult, error) {
	return guard(s.Panics, "Dismiss.Undismiss", func() (UndismissResult, error) {
		if len(req.CommentIDs) == 0 {
			return UndismissResult{}, Errorf(CodeInvalid, "comment_ids are required")
		}
		logger := serviceLogger(s.Log)
		provider, login, err := s.currentAccount(ctx)
		if err != nil {
			return UndismissResult{}, err
		}
		if err := s.Dismissals.Undismiss(ctx, req.CommentIDs, provider, login); err != nil {
			return UndismissResult{}, fmt.Errorf("undismiss failed: %w", err)
		}

		// PR id for the queued action: read from the first comment (best-effort).
		var prID string
		_ = s.DB.QueryRowContext(ctx,
			`SELECT pr_id FROM comments WHERE id = ? LIMIT 1`, req.CommentIDs[0],
		).Scan(&prID)
		s.submitThreadActions(ctx, logger, NormalizeID(prID), actions.UnresolveThreadCommand, req.UnresolveThreadIDs)

		return UndismissResult{UndismissedIDs: req.CommentIDs, UnresolvedThreadIDs: req.UnresolveThreadIDs}, nil
	})
}
