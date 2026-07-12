package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/itterno/bistro/internal/actions"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// DismissalScopeGetter returns the live dismissal scope: "local" (default) or
// "local+provider". Wired to classification settings in increment 4; until
// then app.go passes a constant "local" getter.
type DismissalScopeGetter func() string

// DismissDeps carries the dismiss/undismiss handlers' dependencies.
type DismissDeps struct {
	DB          *sql.DB
	Dismissals  *store.Dismissals
	CurrentUser *store.CurrentUserStore
	// GetApplier returns the current ActionApplier for provider write-back
	// (same contract as SUBMIT_ACTION). Nil applier = write-back silently
	// skipped; the local dismissal always stands.
	GetApplier ApplierGetter
	Scope      DismissalScopeGetter
	Log        *slog.Logger
}

type dismissRequest struct {
	PRID       string   `json:"pr_id"`
	CommentIDs []string `json:"comment_ids"`
}

type undismissRequest struct {
	CommentIDs         []string `json:"comment_ids"`
	UnresolveThreadIDs []string `json:"unresolve_thread_ids"`
}

// currentAccount resolves the account dismissals are namespaced under.
// Falls back to the migration-027 defaults (github/"") when no current_user
// row exists — dismissals still work locally before first auth.
func (d DismissDeps) currentAccount(ctx context.Context) (provider, login string) {
	provider, login = "github", ""
	row := d.DB.QueryRowContext(ctx, `SELECT provider, login FROM current_user ORDER BY provider LIMIT 1`)
	var p, l string
	if err := row.Scan(&p, &l); err == nil {
		provider, login = p, l
	}
	return provider, login
}

// resolvableThreadIDs returns the distinct thread ids among the given
// comments whose stored state is known and unresolved — the only threads a
// provider write-back may resolve (spec: unknown/already-resolved threads
// silently stay local-only).
func (d DismissDeps) resolvableThreadIDs(ctx context.Context, commentIDs []string) ([]string, error) {
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
func (d DismissDeps) submitThreadActions(ctx context.Context, log *slog.Logger, prID string, cmd actions.ActionCommand, threadIDs []string) {
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

// NewDismissCommentsHandler returns the handler for DISMISS_COMMENTS.
// Response: DISMISS_RESULT {batch_id, dismissed_ids, resolved_thread_ids}.
// resolved_thread_ids means "resolve enqueued", not "resolved on GitHub" —
// undo ordering semantics ride the FIFO action queue (spec: Dismissals).
func NewDismissCommentsHandler(deps DismissDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if deps.Log != nil {
			logger = deps.Log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req dismissRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid DISMISS_COMMENTS payload: %w", err)
		}
		if req.PRID == "" || len(req.CommentIDs) == 0 {
			return nil, fmt.Errorf("DISMISS_COMMENTS requires pr_id and comment_ids")
		}

		provider, login := deps.currentAccount(ctx)
		if err := deps.Dismissals.Dismiss(ctx, req.CommentIDs, provider, login); err != nil {
			return nil, fmt.Errorf("dismiss failed: %w", err)
		}

		resolvedThreadIDs := []string{}
		scope := "local"
		if deps.Scope != nil {
			scope = deps.Scope()
		}
		if scope == "local+provider" {
			ids, err := deps.resolvableThreadIDs(ctx, req.CommentIDs)
			if err != nil {
				// Local dismissal stands; write-back is best-effort.
				logger.Warn("resolvable-thread lookup failed; write-back skipped", "err", err)
			} else if len(ids) > 0 {
				deps.submitThreadActions(ctx, logger, req.PRID, actions.ResolveThreadCommand, ids)
				resolvedThreadIDs = ids
			}
		}

		logger.Debug("comments dismissed",
			"pr_id", req.PRID, "count", len(req.CommentIDs), "resolved_threads", len(resolvedThreadIDs))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "DISMISS_RESULT",
			"data": map[string]interface{}{
				"pr_id":               req.PRID,
				"batch_id":            newBatchID(),
				"dismissed_ids":       req.CommentIDs,
				"resolved_thread_ids": resolvedThreadIDs,
			},
		}, nil
	}
}

// NewUndismissCommentsHandler returns the handler for UNDISMISS_COMMENTS.
// UnresolveThread is enqueued for exactly the listed thread ids — never
// inferred — so only threads whose resolve this batch enqueued get undone.
func NewUndismissCommentsHandler(deps DismissDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if deps.Log != nil {
			logger = deps.Log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req undismissRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid UNDISMISS_COMMENTS payload: %w", err)
		}
		if len(req.CommentIDs) == 0 {
			return nil, fmt.Errorf("UNDISMISS_COMMENTS requires comment_ids")
		}

		provider, login := deps.currentAccount(ctx)
		if err := deps.Dismissals.Undismiss(ctx, req.CommentIDs, provider, login); err != nil {
			return nil, fmt.Errorf("undismiss failed: %w", err)
		}

		// PR id for the queued action: read from the first comment (best-effort).
		var prID string
		_ = deps.DB.QueryRowContext(ctx,
			`SELECT pr_id FROM comments WHERE id = ? LIMIT 1`, req.CommentIDs[0],
		).Scan(&prID)
		deps.submitThreadActions(ctx, logger, prID, actions.UnresolveThreadCommand, req.UnresolveThreadIDs)

		logger.Debug("comments undismissed", "count", len(req.CommentIDs))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "UNDISMISS_RESULT",
			"data": map[string]interface{}{
				"undismissed_ids":       req.CommentIDs,
				"unresolved_thread_ids": req.UnresolveThreadIDs,
			},
		}, nil
	}
}
