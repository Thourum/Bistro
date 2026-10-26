package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// RemoveRepoDeps supplies persistence, locking, and cache cleanup.
type RemoveRepoDeps struct {
	DB             *sql.DB
	Store          *store.RepoLocationStore
	Locks          *repolock.Locks
	DeleteCacheDir func(path string) error
	Log            *slog.Logger
}

type removeCounts struct {
	PRs       int `json:"prs"`
	Comments  int `json:"comments"`
	Threads   int `json:"threads"`
	Patchsets int `json:"patchsets"`
	Actions   int `json:"actions"`
}

// NewRemoveRepoPreflightHandler counts rows affected by repository removal.
func NewRemoveRepoPreflightHandler(db *sql.DB, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		repoID, err := repoIDFromMessage(msg, "REMOVE_REPO_PREFLIGHT")
		if err != nil {
			return nil, err
		}
		counts, err := countRepoRows(ctx, db, repoID)
		if err != nil {
			return nil, err
		}
		return locationResponse(msg, struct {
			OK     bool         `json:"ok"`
			Counts removeCounts `json:"counts"`
		}{OK: true, Counts: counts}), nil
	}
}

// NewRemoveRepoHandler removes repository rows and its managed cache checkout.
func NewRemoveRepoHandler(deps RemoveRepoDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		repoID, err := repoIDFromMessage(msg, "REMOVE_REPO")
		if err != nil {
			return nil, err
		}
		// Read location, delete rows, and delete the cache dir under the repo
		// lock so a concurrent clone/recovery cannot race us into deleting a
		// stale path.
		var counts removeCounts
		operation := func() error {
			location, err := deps.Store.Get(ctx, repoID)
			if err != nil {
				return err
			}
			counts, err = removeRepoTx(ctx, deps.DB, repoID)
			if err != nil {
				return err
			}
			if location != nil && location.Source == "cache" && location.LocalPath != "" && deps.DeleteCacheDir != nil {
				if err := deps.DeleteCacheDir(location.LocalPath); err != nil && deps.Log != nil {
					deps.Log.Warn("delete cache dir after remove repo failed", "repo_id", repoID, "error", err)
				}
			}
			return nil
		}
		if deps.Locks != nil {
			err = deps.Locks.WithRepo(repoID, operation)
		} else {
			err = operation()
		}
		if err != nil {
			return nil, err
		}
		return locationResponse(msg, struct {
			OK      bool         `json:"ok"`
			Deleted removeCounts `json:"deleted"`
		}{OK: true, Deleted: counts}), nil
	}
}

type repoRowCounter interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func countRepoRows(ctx context.Context, db repoRowCounter, repoID string) (removeCounts, error) {
	var counts removeCounts
	queries := []struct {
		dest  *int
		query string
	}{
		{&counts.PRs, `SELECT COUNT(*) FROM prs WHERE repo_id = ?`},
		{&counts.Comments, `SELECT COUNT(*) FROM comments WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Threads, `SELECT COUNT(*) FROM review_threads WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Patchsets, `SELECT COUNT(*) FROM patchsets WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Actions, `SELECT COUNT(*) FROM action_queue WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
	}
	for _, query := range queries {
		if err := db.QueryRowContext(ctx, query.query, repoID).Scan(query.dest); err != nil {
			return removeCounts{}, fmt.Errorf("count repo rows %s: %w", repoID, err)
		}
	}
	return counts, nil
}

func removeRepoTx(ctx context.Context, db *sql.DB, repoID string) (removeCounts, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return removeCounts{}, fmt.Errorf("begin remove repo tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			// A rollback after failure may find the transaction already closed.
			_ = tx.Rollback()
		}
	}()
	counts, err := countRepoRows(ctx, tx, repoID)
	if err != nil {
		return removeCounts{}, err
	}
	prSubquery := `SELECT id FROM prs WHERE repo_id = ?`
	deleteStatements := []string{
		`DELETE FROM action_queue WHERE pr_id IN (` + prSubquery + `)`,
		`DELETE FROM patchsets WHERE pr_id IN (` + prSubquery + `)`,
		`DELETE FROM last_reviewed_sha WHERE pr_id IN (` + prSubquery + `)`,
		`DELETE FROM files_viewed WHERE pr_id IN (` + prSubquery + `)`,
		`DELETE FROM prs WHERE repo_id = ?`,
		`DELETE FROM repos WHERE id = ?`,
	}
	for _, statement := range deleteStatements {
		if _, err := tx.ExecContext(ctx, statement, repoID); err != nil {
			return removeCounts{}, fmt.Errorf("remove repo %s: %w", repoID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return removeCounts{}, fmt.Errorf("commit remove repo %s: %w", repoID, err)
	}
	committed = true
	return counts, nil
}
