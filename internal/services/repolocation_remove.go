package services

import (
	"context"
	"database/sql"
	"fmt"
)

type RepoCounts struct {
	PRs       int `json:"prs"`
	Comments  int `json:"comments"`
	Threads   int `json:"threads"`
	Patchsets int `json:"patchsets"`
	Actions   int `json:"actions"`
}

// RemovePreflight counts rows affected by repository removal.
func (s *RepoLocation) RemovePreflight(ctx context.Context, req RepoRef) (RepoCounts, error) {
	return guard(s.Panics, "RepoLocation.RemovePreflight", func() (RepoCounts, error) {
		repoID := NormalizeID(req.RepoID)
		if repoID == "" {
			return RepoCounts{}, Errorf(CodeInvalid, "repo_id is required")
		}
		return countRepoRows(ctx, s.DB, repoID)
	})
}

// Remove deletes repository rows and its managed cache checkout.
func (s *RepoLocation) Remove(ctx context.Context, req RepoRef) (RepoCounts, error) {
	return guard(s.Panics, "RepoLocation.Remove", func() (RepoCounts, error) {
		repoID := NormalizeID(req.RepoID)
		if repoID == "" {
			return RepoCounts{}, Errorf(CodeInvalid, "repo_id is required")
		}

		// Read location, delete rows, and delete the cache dir under the repo
		// lock so a concurrent clone/recovery cannot race us into deleting a
		// stale path.
		var counts RepoCounts
		var err error
		operation := func() error {
			location, err := s.Store.Get(ctx, repoID)
			if err != nil {
				return err
			}
			counts, err = removeRepoTx(ctx, s.DB, repoID)
			if err != nil {
				return err
			}
			if location != nil && location.Source == "cache" && location.LocalPath != "" && s.DeleteCacheDir != nil {
				if err := s.DeleteCacheDir(location.LocalPath); err != nil && s.Log != nil {
					s.Log.Warn("delete cache dir after remove repo failed", "repo_id", repoID, "error", err)
				}
			}
			return nil
		}
		if s.Locks != nil {
			err = s.Locks.WithRepo(repoID, operation)
		} else {
			err = operation()
		}
		if err != nil {
			return RepoCounts{}, err
		}
		return counts, nil
	})
}

type repoRowCounter interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func countRepoRows(ctx context.Context, db repoRowCounter, repoID string) (RepoCounts, error) {
	var counts RepoCounts
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
			return RepoCounts{}, fmt.Errorf("count repo rows %s: %w", repoID, err)
		}
	}
	return counts, nil
}

func removeRepoTx(ctx context.Context, db *sql.DB, repoID string) (RepoCounts, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return RepoCounts{}, fmt.Errorf("begin remove repo tx: %w", err)
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
		return RepoCounts{}, err
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
			return RepoCounts{}, fmt.Errorf("remove repo %s: %w", repoID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return RepoCounts{}, fmt.Errorf("commit remove repo %s: %w", repoID, err)
	}
	committed = true
	return counts, nil
}
