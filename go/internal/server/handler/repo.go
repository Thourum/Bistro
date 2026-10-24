package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	storedb "github.com/itterno/bistro/internal/store/db"
	"github.com/itterno/bistro/internal/ws"
)

type repoRow struct {
	ID                string  `json:"id"`
	Provider          string  `json:"provider"`
	Owner             string  `json:"owner"`
	Repo              string  `json:"repo"`
	Watched           bool    `json:"watched"`
	LocalPath         *string `json:"local_path,omitempty"`
	Source            *string `json:"source,omitempty"`
	LocationStatus    *string `json:"location_status,omitempty"`
	LocationErrorCode *string `json:"location_error_code,omitempty"`
	LocationError     *string `json:"location_error,omitempty"`
}

const repoListTimeout = 30 * time.Second

func repoListErrorResponse(requestID string) map[string]interface{} {
	return map[string]interface{}{
		"id":   requestID,
		"type": "REPO_LIST_READY",
		"data": map[string]interface{}{
			"repos": []repoRow{},
			"error": "unable to load repositories",
		},
	}
}

// NewRequestRepoListHandler returns cached repos as REPO_LIST_READY.
// Unless cache_only is true, it refreshes provider repositories first.
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
		cacheOnly, err := parseRepoListCacheFlag(msg.Data)
		if err != nil {
			logger.Error("invalid repo list payload", "err", err)
			return repoListErrorResponse(msg.ID), nil
		}
		reqCtx, cancel := context.WithTimeout(logging.WithLogger(ctx, logger), repoListTimeout)
		defer cancel()
		queries := storedb.New(db)
		if !cacheOnly {
			prov := getProvider()
			if prov == nil {
				logger.Warn("repo list requested while unauthenticated")
				return map[string]interface{}{"id": msg.ID, "type": "REPO_LIST_READY",
					"data": map[string]interface{}{"repos": []repoRow{}, "error": "not authenticated"}}, nil
			}
			if err := refreshRepoList(reqCtx, queries, prov, logger); err != nil {
				logger.Error("refresh repo list failed", "err", err)
				return repoListErrorResponse(msg.ID), nil
			}
		}
		repos, err := readRepoList(reqCtx, queries)
		if err != nil {
			logger.Error("query repos failed", "err", err)
			return repoListErrorResponse(msg.ID), nil
		}
		logger.Debug("repo list ready", "total", len(repos))
		return map[string]interface{}{"id": msg.ID, "type": "REPO_LIST_READY",
			"data": map[string]interface{}{"repos": repos}}, nil
	}
}

func parseRepoListCacheFlag(payload json.RawMessage) (bool, error) {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return false, nil
	}
	var data struct {
		CacheOnly json.RawMessage `json:"cache_only"`
	}
	if payload[0] != '{' {
		return false, errors.New("repo list payload must be an object")
	}
	if err := json.Unmarshal(payload, &data); err != nil {
		return false, fmt.Errorf("decode repo list payload: %w", err)
	}
	flag := bytes.TrimSpace(data.CacheOnly)
	if len(flag) == 0 || bytes.Equal(flag, []byte("false")) {
		return false, nil
	}
	if bytes.Equal(flag, []byte("true")) {
		return true, nil
	}
	return false, errors.New("cache_only must be a boolean")
}

func refreshRepoList(ctx context.Context, queries *storedb.Queries, prov provider.Provider, logger *slog.Logger) error {
	logger.Debug("listing user repos from provider")
	start := time.Now()
	repos, err := prov.ListUserRepos(ctx)
	if err != nil {
		return fmt.Errorf("list user repos: %w", err)
	}
	logger.Debug("provider returned repos", "count", len(repos), "duration_ms", time.Since(start).Milliseconds())
	for _, repository := range repos {
		if err := queries.InsertDiscoveredRepo(ctx, storedb.InsertDiscoveredRepoParams{
			ID: githubProvider + ":" + repository.FullName, Provider: githubProvider,
			Owner: repository.Owner, Repo: repository.Name,
		}); err != nil {
			return fmt.Errorf("insert provider repo: %w", err)
		}
	}
	logger.Debug("repos upserted", "count", len(repos))
	return nil
}

func readRepoList(ctx context.Context, queries *storedb.Queries) ([]repoRow, error) {
	rows, err := queries.ListOnboardingRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cached repos: %w", err)
	}
	repos := make([]repoRow, 0, len(rows))
	for _, row := range rows {
		repos = append(repos, repoRow{
			ID: row.ID, Provider: row.Provider, Owner: row.Owner, Repo: row.Repo,
			Watched: row.Watched == 1, LocalPath: nullableStringPtr(row.LocalPath),
			Source: nullableStringPtr(row.Source), LocationStatus: nullableStringPtr(row.LocationStatus),
			LocationErrorCode: nullableStringPtr(row.LocationErrorCode), LocationError: nullableStringPtr(row.LocationError),
		})
	}
	return repos, nil
}

func nullableStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("repo not found: %s", data.RepoID)
		}
		if err != nil {
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
