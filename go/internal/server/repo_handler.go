package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/samuelasvanyi/bistro/internal/provider"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

type repoRow struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Watched  bool   `json:"watched"`
}

// newRequestRepoListHandler handles REQUEST_REPO_LIST messages.
// It fetches repos from the provider, upserts them into the DB, then returns
// all repos (with their watched state) as REPO_LIST_READY.
func newRequestRepoListHandler(
	getProvider func() provider.Provider,
	db *sql.DB,
	log *slog.Logger,
) ws.MessageHandler {
	return func(msg ws.InboundMessage) (interface{}, error) {
		prov := getProvider()
		if prov == nil {
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "REPO_LIST_READY",
				"data": map[string]interface{}{
					"repos": []interface{}{},
					"error": "not authenticated",
				},
			}, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		repos, err := prov.ListUserRepos(ctx)
		if err != nil {
			log.Error("ListUserRepos failed", "err", err)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "REPO_LIST_READY",
				"data": map[string]interface{}{
					"repos": []interface{}{},
					"error": err.Error(),
				},
			}, nil
		}

		// Upsert repos into DB, preserving existing watched state.
		for _, r := range repos {
			id := "github:" + r.FullName
			_, upsertErr := db.ExecContext(ctx,
				`INSERT INTO repos (id, provider, owner, repo) VALUES (?, ?, ?, ?)
				 ON CONFLICT(id) DO NOTHING`,
				id, "github", r.Owner, r.Name,
			)
			if upsertErr != nil {
				log.Error("upsert repo failed", "id", id, "err", upsertErr)
			}
		}

		// Return all repos from DB ordered by watched desc, then owner/repo.
		rows, err := db.QueryContext(ctx,
			"SELECT id, provider, owner, repo, watched FROM repos ORDER BY watched DESC, owner ASC, repo ASC",
		)
		if err != nil {
			return nil, fmt.Errorf("query repos: %w", err)
		}
		defer rows.Close()

		var result []repoRow
		for rows.Next() {
			var r repoRow
			var watched int
			if scanErr := rows.Scan(&r.ID, &r.Provider, &r.Owner, &r.Repo, &watched); scanErr != nil {
				log.Error("scan repo row failed", "err", scanErr)
				continue
			}
			r.Watched = watched == 1
			result = append(result, r)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate repos: %w", err)
		}
		if result == nil {
			result = []repoRow{}
		}

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "REPO_LIST_READY",
			"data": map[string]interface{}{
				"repos": result,
			},
		}, nil
	}
}

// newToggleWatchHandler handles TOGGLE_WATCH_REPO messages.
// It flips the watched state for the given repo_id and triggers an immediate
// sync if the repo was just enabled for watching.
func newToggleWatchHandler(
	db *sql.DB,
	syncTrigger chan<- string,
	log *slog.Logger,
) ws.MessageHandler {
	return func(msg ws.InboundMessage) (interface{}, error) {
		var data struct {
			RepoID string `json:"repo_id"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid TOGGLE_WATCH_REPO payload: %w", err)
		}
		if data.RepoID == "" {
			return nil, fmt.Errorf("TOGGLE_WATCH_REPO: repo_id is required")
		}

		// Toggle the watched bit atomically and read back the new state.
		var watched int
		err := db.QueryRow(
			"UPDATE repos SET watched = CASE WHEN watched = 1 THEN 0 ELSE 1 END WHERE id = ? RETURNING watched",
			data.RepoID,
		).Scan(&watched)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("repo not found: %s", data.RepoID)
		}
		if err != nil {
			return nil, fmt.Errorf("toggle watch for %s: %w", data.RepoID, err)
		}

		newWatched := watched == 1

		// Trigger immediate sync if newly watched.
		if newWatched {
			select {
			case syncTrigger <- data.RepoID:
			default:
				log.Warn("sync trigger channel full, skipping immediate sync", "repo_id", data.RepoID)
			}
		}

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "WATCH_REPO_RESULT",
			"data": map[string]interface{}{
				"repo_id": data.RepoID,
				"watched": newWatched,
			},
		}, nil
	}
}
