package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
	storedb "github.com/itterno/bistro/internal/store/db"
)

type RepoRow struct {
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

type RepoListRequest struct {
	CacheOnly bool `json:"cache_only"`
}
type RepoListResponse struct {
	Repos []RepoRow `json:"repos"`
}
type SetWatchedRequest struct {
	RepoID  string `json:"repo_id"`
	Watched *bool  `json:"watched"`
}
type WatchResult struct {
	RepoID  string `json:"repo_id"`
	Watched bool   `json:"watched"`
}

// Repos manages discovery and local watched state.
type Repos struct {
	GetProvider func() provider.Provider
	DB          *sql.DB
	SyncTrigger WatchSyncTrigger
	Panics      PanicReporter
	Log         *slog.Logger
}

// List optionally refreshes the provider, while cache_only never accesses it.
func (s *Repos) List(ctx context.Context, req RepoListRequest) (RepoListResponse, error) {
	return guard(s.Panics, "Repos.List", func() (RepoListResponse, error) {
		logger := serviceLogger(s.Log)
		reqCtx, cancel := context.WithTimeout(logging.WithLogger(ctx, logger), repoListTimeout)
		defer cancel()
		queries := storedb.New(s.DB)
		if !req.CacheOnly {
			var prov provider.Provider
			if s.GetProvider != nil {
				prov = s.GetProvider()
			}
			if prov == nil {
				return RepoListResponse{}, Errorf(CodeUnauthenticated, "not authenticated")
			}
			if err := refreshRepoList(reqCtx, queries, prov, logger); err != nil {
				logger.Error("refresh repo list failed")
				return RepoListResponse{}, Errorf(CodeInternal, "unable to load repositories")
			}
		}
		repos, err := readRepoList(reqCtx, queries)
		if err != nil {
			return RepoListResponse{}, Errorf(CodeInternal, "unable to load repositories")
		}
		return RepoListResponse{Repos: repos}, nil
	})
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
		identity := remote.Identity{Provider: githubProvider, Owner: repository.Owner, Repo: repository.Name}
		if err := queries.InsertDiscoveredRepo(ctx, storedb.InsertDiscoveredRepoParams{
			ID: identity.ID(), Provider: githubProvider,
			Owner: repository.Owner, Repo: repository.Name,
		}); err != nil {
			// One bad row must not hide the cached list from the caller.
			logger.Error("upsert repo failed", "repo_id", identity.ID(), "err", err)
		}
	}
	logger.Debug("repos upserted", "count", len(repos))
	return nil
}

func readRepoList(ctx context.Context, queries *storedb.Queries) ([]RepoRow, error) {
	rows, err := queries.ListOnboardingRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("list cached repos: %w", err)
	}
	repos := make([]RepoRow, 0, len(rows))
	for _, row := range rows {
		repos = append(repos, RepoRow{
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

// WatchSyncTrigger retains a full watched-repo sync when the trigger queue overflows.
type WatchSyncTrigger struct {
	Queue   chan<- string
	Pending *atomic.Bool
}

// SetWatched applies an explicit watched state, or toggles when watched is omitted.
func (s *Repos) SetWatched(ctx context.Context, req SetWatchedRequest) (WatchResult, error) {
	return guard(s.Panics, "Repos.SetWatched", func() (WatchResult, error) {
		req.RepoID = NormalizeID(req.RepoID)
		logger := serviceLogger(s.Log)
		if req.RepoID == "" {
			return WatchResult{}, Errorf(CodeInvalid, "repo_id is required")
		}

		logger.Debug("toggle watch requested", "repo_id", req.RepoID)

		var currentWatched int
		err := s.DB.QueryRowContext(ctx, "SELECT watched FROM repos WHERE id = ?", req.RepoID).Scan(&currentWatched)
		if errors.Is(err, sql.ErrNoRows) {
			return WatchResult{}, Errorf(CodeNotFound, "repo not found: %s", req.RepoID)
		}
		if err != nil {
			return WatchResult{}, fmt.Errorf("read watch state for %s: %w", req.RepoID, err)
		}

		wasWatched := currentWatched == 1
		newWatched := !wasWatched
		if req.Watched != nil {
			newWatched = *req.Watched
		}

		watchedValue := 0
		if newWatched {
			watchedValue = 1
		}

		if _, err := s.DB.ExecContext(ctx, "UPDATE repos SET watched = ? WHERE id = ?", watchedValue, req.RepoID); err != nil {
			return WatchResult{}, fmt.Errorf("toggle watch for %s: %w", req.RepoID, err)
		}

		logger.Info("watch state toggled", "repo_id", req.RepoID, "watched", newWatched)

		if newWatched && !wasWatched {
			select {
			case s.SyncTrigger.Queue <- req.RepoID:
				logger.Debug("sync triggered", "repo_id", req.RepoID)
			default:
				s.SyncTrigger.Pending.Store(true)
				// Wake the loop if it drained the queue before the flag was stored.
				select {
				case s.SyncTrigger.Queue <- "":
				default:
				}
				logger.Debug("sync trigger channel full, watched-repo sync pending", "repo_id", req.RepoID)
			}
		}

		return WatchResult{RepoID: req.RepoID, Watched: newWatched}, nil
	})
}
