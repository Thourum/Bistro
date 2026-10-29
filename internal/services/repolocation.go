package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	gogit "github.com/go-git/go-git/v5"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/git/repolock"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store/repolocation"
)

const githubRepoURLFormat = "https://github.com/%s/%s.git"

// RepoLocation manages local checkouts and the clone cache.
type RepoLocation struct {
	DB             *sql.DB
	Store          *repolocation.Store
	Clone          func(context.Context, string, string, bool) error
	CacheRoot      func() (string, error)
	Locks          *repolock.Locks
	DeleteCacheDir func(string) error
	SweepOrphans   func(map[string]bool) ([]string, error)
	Panics         PanicReporter
	Log            *slog.Logger
}

// locationError maps a repo-location failure code onto the closed Code set,
// keeping the original code as Reason for the UI.
func locationError(code, message string) *ServiceError {
	c := CodeInternal
	switch code {
	case "invalid_url", "destination_required", "invalid_path", "destination_conflict", "remote_mismatch", "path_missing":
		c = CodeInvalid
	case "auth_failed":
		c = CodeUnauthenticated
	case "cancelled":
		c = CodeCancelled
	}
	return WithReason(c, code, message)
}

func updateScannedRepoLocations(ctx context.Context, locations *repolocation.Store, found []scan.Found) (int, int, error) {
	for _, repo := range found {
		if err := locations.UpsertLocal(ctx, repo.Identity.Provider, repo.Identity.Owner, repo.Identity.Repo, repo.Path, false); err != nil {
			return 0, 0, fmt.Errorf("upsert scanned repo %s: %w", repo.Identity.ID(), err)
		}
	}
	localRows, err := locations.ListBySource(ctx, "local")
	if err != nil {
		return 0, 0, fmt.Errorf("list local repo locations: %w", err)
	}
	var healthy, degraded int
	for _, row := range localRows {
		status, code, message := healthCheckLocalRepo(ctx, row)
		if status == "ok" {
			if err := locations.ClearDegraded(ctx, row.RepoID); err != nil {
				return 0, 0, fmt.Errorf("clear degraded %s: %w", row.RepoID, err)
			}
			healthy++
			continue
		}
		if err := locations.MarkDegraded(ctx, row.RepoID, status, code, message); err != nil {
			return 0, 0, fmt.Errorf("mark degraded %s: %w", row.RepoID, err)
		}
		degraded++
	}
	return healthy, degraded, nil
}

func isBistroRepoDirOverrideSet() bool {
	return os.Getenv("BISTRO_REPO_DIR") != ""
}

func triggerSync(syncTrigger chan<- string, logger *slog.Logger) {
	if syncTrigger == nil {
		return
	}
	select {
	case syncTrigger <- "":
	default:
		if logger != nil {
			logger.Warn("sync trigger channel full, skipping repo location sync trigger")
		}
	}
}

func healthCheckLocalRepo(ctx context.Context, row repolocation.Location) (string, string, string) {
	info, err := os.Stat(row.LocalPath)
	if err != nil {
		logging.FromContext(ctx).Warn("local repo path stat failed", "error", err)
		if os.IsNotExist(err) {
			return "missing", "path_missing", "repository path not found"
		}
		if os.IsPermission(err) {
			return "permission_denied", "permission_denied", "repository path is not readable"
		}
		return "missing", "path_missing", "repository path not found"
	}
	if !info.IsDir() {
		return "missing", "path_missing", "path is not a directory"
	}
	if ok, err := canReadDir(row.LocalPath); !ok {
		logging.FromContext(ctx).Warn("local repo path unreadable", "error", err)
		return "permission_denied", "permission_denied", "repository path is not readable"
	}
	identity := remote.Identity{Provider: row.Provider, Owner: row.Owner, Repo: row.Repo}
	if !repoPathMatches(row.LocalPath, identity) {
		return "remote_mismatch", "remote_mismatch", fmt.Sprintf("no remote matches %s", identity.ID())
	}
	return "ok", "", ""
}

func canReadDir(path string) (bool, error) {
	dir, err := os.Open(path)
	if err != nil {
		if os.IsPermission(err) {
			return false, err
		}
		return true, err
	}
	// A read-only directory handle has no pending writes to flush on close.
	defer func() { _ = dir.Close() }()

	_, err = dir.Readdirnames(1)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if errors.Is(err, os.ErrClosed) {
		return false, err
	}
	if errors.Is(err, os.ErrPermission) || os.IsPermission(err) {
		return false, err
	}
	return true, err
}

func repoPathMatches(path string, identity remote.Identity) bool {
	matches, err := repoPathRemoteMatches(path, identity)
	return err == nil && matches
}

func repoPathRemoteMatches(path string, identity remote.Identity) (bool, error) {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return false, fmt.Errorf("open git repo %q: %w", path, err)
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return false, fmt.Errorf("read remotes for %q: %w", path, err)
	}
	for _, repoRemote := range remotes {
		for _, rawURL := range repoRemote.Config().URLs {
			remoteID, err := remote.ParseURL(rawURL)
			if err == nil && remoteID.ID() == identity.ID() {
				return true, nil
			}
		}
	}
	return false, nil
}
