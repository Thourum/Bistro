package services

import (
	"context"
	"errors"
	"time"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/store/repolocation"
)

type CacheFailure struct {
	RepoID  string `json:"repo_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CacheRepoStat struct {
	RepoID     string     `json:"repo_id"`
	SizeBytes  int64      `json:"size_bytes"`
	LastUsedAt *time.Time `json:"last_used_at"`
	Stale      bool       `json:"stale"`
}

type CacheStats struct {
	TotalBytes  int64           `json:"total_bytes"`
	ActiveBytes int64           `json:"active_bytes"`
	StaleBytes  int64           `json:"stale_bytes"`
	Repos       []CacheRepoStat `json:"repos"`
}
type CacheCleanup struct {
	Removed        []string       `json:"removed"`
	Failed         []CacheFailure `json:"failed"`
	OrphansRemoved int            `json:"orphans_removed"`
}

// CacheStats summarizes managed cache usage.
func (s *RepoLocation) CacheStats(ctx context.Context) (CacheStats, error) {
	return guard(s.Panics, "RepoLocation.CacheStats", func() (CacheStats, error) {
		stats, err := cacheStats(ctx, s.Store)
		if err != nil {
			return CacheStats{}, err
		}
		out := CacheStats{Repos: make([]CacheRepoStat, 0, len(stats))}
		for _, stat := range stats {
			if stat.Skipped {
				continue
			}
			out.TotalBytes += stat.SizeBytes
			if stat.Stale {
				out.StaleBytes += stat.SizeBytes
			} else {
				out.ActiveBytes += stat.SizeBytes
			}
			out.Repos = append(out.Repos, CacheRepoStat{RepoID: stat.RepoID, SizeBytes: stat.SizeBytes, LastUsedAt: stat.LastUsedAt, Stale: stat.Stale})
		}
		return out, nil
	})
}

// PruneCache removes stale checkouts, retaining individual failures as data.
func (s *RepoLocation) PruneCache(ctx context.Context) (CacheCleanup, error) {
	return guard(s.Panics, "RepoLocation.PruneCache", func() (CacheCleanup, error) {
		var out CacheCleanup
		err := runCacheMaintenance(s.Locks, func() error {
			var err error
			out.Removed, out.Failed, err = pruneCache(ctx, s.Store, requireDeleteCacheDir(s.DeleteCacheDir))
			return err
		})
		if err != nil {
			if ctxErr := contextError(err); ctxErr != nil {
				return out, ctxErr
			}
			serviceLogger(s.Log).Warn("prune cache failed", "error", err)
			return out, locationError("delete_failed", "unable to prune cache")
		}
		return out, nil
	})
}

// ClearCache deletes managed checkouts and sweeps orphans.
func (s *RepoLocation) ClearCache(ctx context.Context) (CacheCleanup, error) {
	return guard(s.Panics, "RepoLocation.ClearCache", func() (CacheCleanup, error) {
		var out CacheCleanup
		err := runCacheMaintenance(s.Locks, func() error {
			var err error
			out.Removed, out.Failed, out.OrphansRemoved, err = clearCache(ctx, s.Store, requireDeleteCacheDir(s.DeleteCacheDir), s.SweepOrphans)
			return err
		})
		if err != nil {
			if ctxErr := contextError(err); ctxErr != nil {
				return out, ctxErr
			}
			serviceLogger(s.Log).Warn("clear cache failed", "error", err)
			return out, locationError("delete_failed", "unable to clear cache")
		}
		return out, nil
	})
}

// DeleteCacheRepo clears the managed checkout for one repository.
func (s *RepoLocation) DeleteCacheRepo(ctx context.Context, req RepoRef) error {
	return guardErr(s.Panics, "RepoLocation.DeleteCacheRepo", func() error {
		repoID := NormalizeID(req.RepoID)
		if repoID == "" {
			return Errorf(CodeInvalid, "repo_id is required")
		}
		location, err := s.Store.Get(ctx, repoID)
		if err != nil {
			return err
		}
		if location == nil {
			return Errorf(CodeNotFound, "repo not found")
		}
		if location.Source != "cache" {
			return locationError("delete_failed", "repo is not a cache repo")
		}
		operation := func() error {
			lockedLocation, err := s.Store.Get(ctx, repoID)
			if err != nil {
				return err
			}
			if lockedLocation == nil || lockedLocation.Source != "cache" || lockedLocation.LocalPath == "" {
				return errors.New("repo is no longer a cache repo")
			}
			if err := validateCacheRepoPathInsideRoot(lockedLocation.LocalPath); err != nil {
				return err
			}
			if err := requireDeleteCacheDir(s.DeleteCacheDir)(lockedLocation.LocalPath); err != nil {
				return err
			}
			return s.Store.ClearCache(ctx, repoID)
		}
		if s.Locks != nil {
			err = s.Locks.WithRepo(repoID, operation)
		} else {
			err = operation()
		}
		if err != nil {
			if ctxErr := contextError(err); ctxErr != nil {
				return ctxErr
			}
			serviceLogger(s.Log).Warn("delete cache repo failed", "error", err)
			return locationError("delete_failed", "unable to delete cache repository")
		}
		return nil
	})
}

func runCacheMaintenance(locks *repolock.Locks, operation func() error) error {
	if locks != nil {
		return locks.WithMaintenance(operation)
	}
	return operation()
}

func cacheStats(ctx context.Context, locations *repolocation.Store) ([]clonecache.RepoStat, error) {
	cacheRows, err := locations.ListBySource(ctx, "cache")
	if err != nil {
		return nil, err
	}
	inputs := make([]clonecache.EntryInput, 0, len(cacheRows))
	for _, row := range cacheRows {
		inputs = append(inputs, clonecache.EntryInput{RepoID: row.RepoID, Path: row.LocalPath, LastUsedAt: row.LastUsedAt})
	}
	return clonecache.Stats(inputs)
}

func requireDeleteCacheDir(deleteCacheDir func(path string) error) func(path string) error {
	if deleteCacheDir != nil {
		return deleteCacheDir
	}
	return func(string) error {
		return errors.New("delete cache dir dependency is not configured")
	}
}

func pruneCache(ctx context.Context, locations *repolocation.Store, deleteCacheDir func(path string) error) ([]string, []CacheFailure, error) {
	stats, err := cacheStats(ctx, locations)
	if err != nil {
		return nil, nil, err
	}
	removed := []string{}
	failed := []CacheFailure{}
	for _, stat := range stats {
		if !stat.Stale {
			continue
		}
		if err := deleteCacheDir(stat.Path); err != nil {
			logging.FromContext(ctx).Warn("prune cache entry failed", "repo_id", stat.RepoID, "error", err)
			failed = append(failed, CacheFailure{RepoID: stat.RepoID, Code: "delete_failed", Message: "unable to delete cache repository"})
			continue
		}
		if err := locations.ClearCache(ctx, stat.RepoID); err != nil {
			return nil, nil, err
		}
		removed = append(removed, stat.RepoID)
	}
	return removed, failed, nil
}

func clearCache(ctx context.Context, locations *repolocation.Store, deleteCacheDir func(path string) error, sweepOrphans func(known map[string]bool) ([]string, error)) ([]string, []CacheFailure, int, error) {
	cacheRows, err := locations.ListBySource(ctx, "cache")
	if err != nil {
		return nil, nil, 0, err
	}
	known := make(map[string]bool, len(cacheRows))
	removed := []string{}
	failed := []CacheFailure{}
	for _, row := range cacheRows {
		known[row.LocalPath] = true
		if err := deleteCacheDir(row.LocalPath); err != nil {
			logging.FromContext(ctx).Warn("clear cache entry failed", "repo_id", row.RepoID, "error", err)
			failed = append(failed, CacheFailure{RepoID: row.RepoID, Code: "delete_failed", Message: "unable to delete cache repository"})
			continue
		}
		if err := locations.ClearCache(ctx, row.RepoID); err != nil {
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
			failed = append(failed, CacheFailure{Code: "delete_failed", Message: "unable to sweep cache orphans"})
		}
	}
	return removed, failed, orphansRemoved, nil
}
