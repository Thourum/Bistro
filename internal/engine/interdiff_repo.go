package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/repolocation"
)

// prRepoLocator finds the repo a PR's diffs were generated in: the user's
// local clone or the cache clone, whichever the sync recorded.
type prRepoLocator struct {
	db        *store.DB
	locations *repolocation.Store
}

// RepoForPR returns the sync's clone URL and recorded local repository path.
func (l prRepoLocator) RepoForPR(ctx context.Context, prID string) (string, string, bool, error) {
	var owner, repo string
	err := l.db.QueryRowContext(ctx, `SELECT owner, repo FROM prs WHERE id = ?`, prID).Scan(&owner, &repo)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("look up repo of PR %s: %w", prID, err)
	}
	location, err := l.locations.Get(ctx, remote.Identity{Provider: "github", Owner: owner, Repo: repo}.ID())
	if err != nil {
		return "", "", false, fmt.Errorf("get repo location of PR %s: %w", prID, err)
	}
	if location == nil || location.LocalPath == "" {
		return "", "", false, nil
	}
	// Same URL the sync clones (sync/fetcher.go), so the repo lock matches.
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, repo), location.LocalPath, true, nil
}
