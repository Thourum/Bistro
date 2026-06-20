package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	"github.com/itterno/bistro/internal/ws"
)

type repoRow struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
	Watched  bool   `json:"watched"`
}

// NewRequestRepoListHandler handles REQUEST_REPO_LIST messages.
// It fetches repos from the provider, upserts them into the DB, then returns
// all repos (with their watched state) as REPO_LIST_READY.
func NewRequestRepoListHandler(
	getProvider func() provider.Provider,
	db *sql.DB,
	log *slog.Logger,
) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		prov := getProvider()
		if prov == nil {
			logger.Warn("repo list requested while unauthenticated")
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "REPO_LIST_READY",
				"data": map[string]interface{}{
					"repos": []interface{}{},
					"error": "not authenticated",
				},
			}, nil
		}

		reqCtx, cancel := context.WithTimeout(logging.WithLogger(ctx, logger), 30*time.Second)
		defer cancel()

		logger.Debug("listing user repos from provider")
		start := time.Now()
		repos, err := prov.ListUserRepos(reqCtx)
		if err != nil {
			logger.Error("ListUserRepos failed",
				"duration_ms", time.Since(start).Milliseconds(),
				"err", err,
			)
			return map[string]interface{}{
				"id":   msg.ID,
				"type": "REPO_LIST_READY",
				"data": map[string]interface{}{
					"repos": []interface{}{},
					"error": err.Error(),
				},
			}, nil
		}
		logger.Debug("provider returned repos",
			"count", len(repos),
			"duration_ms", time.Since(start).Milliseconds(),
		)

		const provName = "github"
		upserts := 0
		for _, r := range repos {
			id := provName + ":" + r.FullName
			_, upsertErr := db.ExecContext(reqCtx,
				`INSERT INTO repos (id, provider, owner, repo) VALUES (?, ?, ?, ?)
				 ON CONFLICT(id) DO NOTHING`,
				id, provName, r.Owner, r.Name,
			)
			if upsertErr != nil {
				logger.Error("upsert repo failed", "id", id, "err", upsertErr)
				continue
			}
			upserts++
		}
		logger.Debug("repos upserted", "attempted", len(repos), "succeeded", upserts)

		rows, err := db.QueryContext(reqCtx,
			"SELECT id, provider, owner, repo, watched FROM repos ORDER BY watched DESC, owner ASC, repo ASC",
		)
		if err != nil {
			logger.Error("query repos failed", "err", err)
			return nil, fmt.Errorf("query repos: %w", err)
		}
		defer rows.Close()

		var result []repoRow
		for rows.Next() {
			var r repoRow
			var watched int
			if scanErr := rows.Scan(&r.ID, &r.Provider, &r.Owner, &r.Repo, &watched); scanErr != nil {
				logger.Error("scan repo row failed", "err", scanErr)
				continue
			}
			r.Watched = watched == 1
			result = append(result, r)
		}
		if err := rows.Err(); err != nil {
			logger.Error("iterate repos failed", "err", err)
			return nil, fmt.Errorf("iterate repos: %w", err)
		}
		if result == nil {
			result = []repoRow{}
		}

		logger.Debug("repo list ready", "total", len(result))

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "REPO_LIST_READY",
			"data": map[string]interface{}{
				"repos": result,
			},
		}, nil
	}
}

// NewToggleWatchHandler handles TOGGLE_WATCH_REPO messages.
// It flips the watched state for the given repo_id and triggers an immediate
// sync if the repo was just enabled for watching.
func NewToggleWatchHandler(
	db *sql.DB,
	syncTrigger chan<- string,
	log *slog.Logger,
) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var data struct {
			RepoID  string `json:"repo_id"`
			Watched *bool  `json:"watched"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid TOGGLE_WATCH_REPO payload: %w", err)
		}
		if data.RepoID == "" {
			return nil, fmt.Errorf("TOGGLE_WATCH_REPO: repo_id is required")
		}

		logger.Debug("toggle watch requested", "repo_id", data.RepoID)

		var currentWatched int
		err := db.QueryRow("SELECT watched FROM repos WHERE id = ?", data.RepoID).Scan(&currentWatched)
		if err == sql.ErrNoRows {
			logger.Warn("toggle watch repo not found", "repo_id", data.RepoID)
			return nil, fmt.Errorf("repo not found: %s", data.RepoID)
		}
		if err != nil {
			logger.Error("read watch state failed", "repo_id", data.RepoID, "err", err)
			return nil, fmt.Errorf("read watch state for %s: %w", data.RepoID, err)
		}

		wasWatched := currentWatched == 1
		newWatched := !wasWatched
		if data.Watched != nil {
			newWatched = *data.Watched
		}

		watchedValue := 0
		if newWatched {
			watchedValue = 1
		}

		if _, err := db.Exec("UPDATE repos SET watched = ? WHERE id = ?", watchedValue, data.RepoID); err != nil {
			logger.Error("toggle watch failed", "repo_id", data.RepoID, "err", err)
			return nil, fmt.Errorf("toggle watch for %s: %w", data.RepoID, err)
		}

		logger.Info("watch state toggled", "repo_id", data.RepoID, "watched", newWatched)

		if newWatched && !wasWatched {
			select {
			case syncTrigger <- data.RepoID:
				logger.Debug("sync triggered", "repo_id", data.RepoID)
			default:
				logger.Warn("sync trigger channel full, skipping immediate sync", "repo_id", data.RepoID)
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
