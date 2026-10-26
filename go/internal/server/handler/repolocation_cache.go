package handler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// CacheMaintenanceDeps supplies cache persistence, locking, and deletion.
type CacheMaintenanceDeps struct {
	Store          *store.RepoLocationStore
	Locks          *repolock.Locks
	DeleteCacheDir func(path string) error
	SweepOrphans   func(known map[string]bool) ([]string, error)
	Log            *slog.Logger
}

type cacheFailurePayload struct {
	RepoID  string `json:"repo_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type cacheRepoStatPayload struct {
	RepoID     string     `json:"repo_id"`
	SizeBytes  int64      `json:"size_bytes"`
	LastUsedAt *time.Time `json:"last_used_at"`
	Stale      bool       `json:"stale"`
}

// NewGetCacheStatsHandler summarizes managed cache usage.
func NewGetCacheStatsHandler(deps CacheMaintenanceDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		stats, err := cacheStats(ctx, deps.Store)
		if err != nil {
			return nil, err
		}
		repos := make([]cacheRepoStatPayload, 0, len(stats))
		var total, active, stale int64
		for _, stat := range stats {
			if stat.Skipped {
				continue
			}
			total += stat.SizeBytes
			if stat.Stale {
				stale += stat.SizeBytes
			} else {
				active += stat.SizeBytes
			}
			repos = append(repos, cacheRepoStatPayload{RepoID: stat.RepoID, SizeBytes: stat.SizeBytes, LastUsedAt: stat.LastUsedAt, Stale: stat.Stale})
		}
		return locationResponse(msg, struct {
			OK          bool                   `json:"ok"`
			TotalBytes  int64                  `json:"total_bytes"`
			ActiveBytes int64                  `json:"active_bytes"`
			StaleBytes  int64                  `json:"stale_bytes"`
			Repos       []cacheRepoStatPayload `json:"repos"`
		}{OK: true, TotalBytes: total, ActiveBytes: active, StaleBytes: stale, Repos: repos}), nil
	}
}

// NewPruneCacheHandler removes stale cache checkouts.
func NewPruneCacheHandler(deps CacheMaintenanceDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var payload any
		operation := func() error {
			removed, failed, err := pruneCache(ctx, deps.Store, requireDeleteCacheDir(deps.DeleteCacheDir))
			if err != nil {
				return err
			}
			payload = cacheMutationPayload{OK: true, Removed: removed, Failed: failed}
			return nil
		}
		if err := runCacheMaintenance(deps.Locks, operation); err != nil {
			repoLocationLogger(ctx, deps.Log, msg).Warn("prune cache failed", "error", err)
			return locationResponse(msg, locationError("", operationPruneCache, "delete_failed", "unable to prune cache")), nil
		}
		return locationResponse(msg, payload), nil
	}
}

// NewClearCacheHandler deletes managed cache checkouts and sweeps orphans.
func NewClearCacheHandler(deps CacheMaintenanceDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var payload any
		operation := func() error {
			removed, failed, orphansRemoved, err := clearCache(ctx, deps.Store, requireDeleteCacheDir(deps.DeleteCacheDir), deps.SweepOrphans)
			if err != nil {
				return err
			}
			payload = cacheMutationPayload{OK: true, Removed: removed, Failed: failed, OrphansRemoved: orphansRemoved}
			return nil
		}
		if err := runCacheMaintenance(deps.Locks, operation); err != nil {
			repoLocationLogger(ctx, deps.Log, msg).Warn("clear cache failed", "error", err)
			return locationResponse(msg, locationError("", operationClearCache, "delete_failed", "unable to clear cache")), nil
		}
		return locationResponse(msg, payload), nil
	}
}

// NewDeleteCacheRepoHandler clears the managed checkout for one repository.
func NewDeleteCacheRepoHandler(deps CacheMaintenanceDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		repoID, err := repoIDFromMessage(msg, "DELETE_CACHE_REPO")
		if err != nil {
			return nil, err
		}
		location, err := deps.Store.Get(ctx, repoID)
		if err != nil {
			return nil, err
		}
		if location == nil || location.Source != "cache" {
			return locationResponse(msg, locationError(repoID, operationDeleteCacheRepo, "delete_failed", "repo is not a cache repo")), nil
		}
		operation := func() error {
			lockedLocation, err := deps.Store.Get(ctx, repoID)
			if err != nil {
				return err
			}
			if lockedLocation == nil || lockedLocation.Source != "cache" || lockedLocation.LocalPath == "" {
				return errors.New("repo is no longer a cache repo")
			}
			if err := validateCacheRepoPathInsideRoot(lockedLocation.LocalPath); err != nil {
				return err
			}
			if err := requireDeleteCacheDir(deps.DeleteCacheDir)(lockedLocation.LocalPath); err != nil {
				return err
			}
			return deps.Store.ClearCacheLocation(ctx, repoID)
		}
		if deps.Locks != nil {
			if err := deps.Locks.WithRepo(repoID, operation); err != nil {
				repoLocationLogger(ctx, deps.Log, msg).Warn("delete cache repo failed", "error", err)
				return locationResponse(msg, locationError(repoID, operationDeleteCacheRepo, "delete_failed", "unable to delete cache repository")), nil
			}
		} else if err := operation(); err != nil {
			repoLocationLogger(ctx, deps.Log, msg).Warn("delete cache repo failed", "error", err)
			return locationResponse(msg, locationError(repoID, operationDeleteCacheRepo, "delete_failed", "unable to delete cache repository")), nil
		}
		return locationResponse(msg, struct {
			OK bool `json:"ok"`
		}{OK: true}), nil
	}
}

func runCacheMaintenance(locks *repolock.Locks, operation func() error) error {
	if locks != nil {
		return locks.WithMaintenance(operation)
	}
	return operation()
}

func cacheStats(ctx context.Context, locations *store.RepoLocationStore) ([]git.CacheRepoStat, error) {
	cacheRows, err := locations.ListBySource(ctx, "cache")
	if err != nil {
		return nil, err
	}
	inputs := make([]git.CacheEntryInput, 0, len(cacheRows))
	for _, row := range cacheRows {
		inputs = append(inputs, git.CacheEntryInput{RepoID: row.RepoID, Path: row.LocalPath, LastUsedAt: row.LastUsedAt})
	}
	return git.CacheStats(inputs)
}

func requireDeleteCacheDir(deleteCacheDir func(path string) error) func(path string) error {
	if deleteCacheDir != nil {
		return deleteCacheDir
	}
	return func(string) error {
		return errors.New("delete cache dir dependency is not configured")
	}
}

func pruneCache(ctx context.Context, locations *store.RepoLocationStore, deleteCacheDir func(path string) error) ([]string, []cacheFailurePayload, error) {
	stats, err := cacheStats(ctx, locations)
	if err != nil {
		return nil, nil, err
	}
	removed := []string{}
	failed := []cacheFailurePayload{}
	for _, stat := range stats {
		if !stat.Stale {
			continue
		}
		if err := deleteCacheDir(stat.Path); err != nil {
			logging.FromContext(ctx).Warn("prune cache entry failed", "repo_id", stat.RepoID, "error", err)
			failed = append(failed, cacheFailurePayload{RepoID: stat.RepoID, Code: "delete_failed", Message: "unable to delete cache repository"})
			continue
		}
		if err := locations.ClearCacheLocation(ctx, stat.RepoID); err != nil {
			return nil, nil, err
		}
		removed = append(removed, stat.RepoID)
	}
	return removed, failed, nil
}

func clearCache(ctx context.Context, locations *store.RepoLocationStore, deleteCacheDir func(path string) error, sweepOrphans func(known map[string]bool) ([]string, error)) ([]string, []cacheFailurePayload, int, error) {
	cacheRows, err := locations.ListBySource(ctx, "cache")
	if err != nil {
		return nil, nil, 0, err
	}
	known := make(map[string]bool, len(cacheRows))
	removed := []string{}
	failed := []cacheFailurePayload{}
	for _, row := range cacheRows {
		known[row.LocalPath] = true
		if err := deleteCacheDir(row.LocalPath); err != nil {
			logging.FromContext(ctx).Warn("clear cache entry failed", "repo_id", row.RepoID, "error", err)
			failed = append(failed, cacheFailurePayload{RepoID: row.RepoID, Code: "delete_failed", Message: "unable to delete cache repository"})
			continue
		}
		if err := locations.ClearCacheLocation(ctx, row.RepoID); err != nil {
			return nil, nil, 0, err
		}
		removed = append(removed, row.RepoID)
	}
	orphansRemoved := 0
	if sweepOrphans != nil {
		// A sweep failure must not discard the cache-row deletions that already
		// succeeded above; report it as a failure entry instead of an error.
		orphans, err := sweepOrphans(known)
		orphansRemoved = len(orphans)
		if err != nil {
			logging.FromContext(ctx).Warn("cache orphan sweep failed", "error", err)
			failed = append(failed, cacheFailurePayload{Code: "delete_failed", Message: "unable to sweep cache orphans"})
		}
	}
	return removed, failed, orphansRemoved, nil
}

type cacheMutationPayload struct {
	OK             bool                  `json:"ok"`
	Removed        []string              `json:"removed"`
	Failed         []cacheFailurePayload `json:"failed"`
	OrphansRemoved int                   `json:"orphans_removed"`
}
