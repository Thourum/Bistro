package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store/repolocation"
)

// RecoverPath restores a repository location.
func (s *RepoLocation) RecoverPath(ctx context.Context, req RecoverPathRequest) error {
	return guardErr(s.Panics, "RepoLocation.RecoverPath", func() error {
		req.RepoID = NormalizeID(req.RepoID)
		if req.RepoID == "" {
			return Errorf(CodeInvalid, "repo_id is required")
		}

		location, err := s.Store.Get(ctx, req.RepoID)
		if err != nil {
			return err
		}
		if location == nil {
			return Errorf(CodeNotFound, "repo not found")
		}
		canonical, err := filepath.EvalSymlinks(req.Path)
		if err != nil {
			serviceLogger(s.Log).Warn("recover repository path failed", "error", err)
			if ctxErr := contextError(err); ctxErr != nil {
				return ctxErr
			}
			return locationError(recoverPathErrorCode(err), "unable to read repository path")
		}
		matches, err := repoPathRemoteMatches(canonical, remote.Identity{Provider: location.Provider, Owner: location.Owner, Repo: location.Repo})
		if err != nil {
			serviceLogger(s.Log).Warn("recover repository path failed", "error", err)
			if ctxErr := contextError(err); ctxErr != nil {
				return ctxErr
			}
			return locationError(recoverPathErrorCode(err), "unable to read repository path")
		}
		if !matches {
			return locationError(locationRemoteMismatch, "path remote does not match repo")
		}
		if err := s.Store.UpsertLocal(ctx, location.Provider, location.Owner, location.Repo, canonical, false); err != nil {
			return err
		}
		return nil
	})
}

// RecoverToCache restores a repository location.
func (s *RepoLocation) RecoverToCache(ctx context.Context, req RepoRef) error {
	return guardErr(s.Panics, "RepoLocation.RecoverToCache", func() error {
		req.RepoID = NormalizeID(req.RepoID)
		if req.RepoID == "" {
			return Errorf(CodeInvalid, "repo_id is required")
		}
		repoID := req.RepoID

		location, err := s.Store.Get(ctx, repoID)
		if err != nil {
			return err
		}
		if location == nil {
			return Errorf(CodeNotFound, "repo not found")
		}
		identity := remote.Identity{Provider: location.Provider, Owner: location.Owner, Repo: location.Repo}
		root, err := s.CacheRoot()
		if err != nil {
			return err
		}
		destination := filepath.Join(root, identity.Owner, identity.Repo)
		state, err := inspectDestination(destination, identity)
		if err != nil {
			if ctxErr := contextError(err); ctxErr != nil {
				return ctxErr
			}
			serviceLogger(s.Log).Warn("inspect cache destination failed", "error", err)
			return locationError(locationCloneFailed, "unable to prepare cache repository")
		}
		if state.Matches {
			if err := s.Store.SetCache(ctx, repoID, destination); err != nil {
				return err
			}
			return nil
		}
		if !state.CanClone {
			return locationError(locationCloneFailed, state.Message)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		createdByBistro, err := prepareCloneDestination(destination)
		if err != nil {
			return err
		}
		repoURL := remote.GitHubCloneURL(identity.Owner, identity.Repo)
		if err := s.cloneAndVerify(ctx, repoCloneDestination{
			URL: repoURL, Path: destination, Identity: identity,
			Source: "cache", Root: root, CreatedByBistro: createdByBistro,
			FailureLog:     "recover cache clone failed",
			MismatchReason: locationCloneFailed, MismatchMessage: "cloned repository remote does not match repo",
		}); err != nil {
			return err
		}
		if err := s.Store.SetCache(ctx, repoID, destination); err != nil {
			return err
		}
		return nil
	})
}

func destinationRootErrorCode(err error) string {
	if errors.Is(err, errDevFolderPathRequired) {
		return locationDestinationRequired
	}
	if errors.Is(err, errDevFolderPathInvalid) {
		return locationInvalidPath
	}
	return locationDestinationConflict
}

func recoverPathErrorCode(err error) string {
	if os.IsPermission(err) || errors.Is(err, os.ErrPermission) {
		return "permission_denied"
	}
	return locationPathMissing
}

func registeredLocal(ctx context.Context, locations *repolocation.Store, repoID string) (bool, *ServiceError, error) {
	location, err := locations.Get(ctx, repoID)
	if err != nil {
		return false, nil, fmt.Errorf("get repo location %s: %w", repoID, err)
	}
	if location == nil || location.Source != "local" {
		return false, nil, nil
	}
	if location.LocationStatus == "ok" {
		return true, nil, nil
	}
	code := location.LocationErrorCode
	if code == "" {
		code = locationPathMissing
	}
	message := location.LocationError
	if message == "" {
		message = fmt.Sprintf("local repository is degraded: %s", location.LocationStatus)
	}
	return false, locationError(code, message), nil
}

func targetedDevFolder(ctx context.Context, locations *repolocation.Store, identity remote.Identity, devFolder string) (bool, error) {
	canonicalDev, err := filepath.EvalSymlinks(devFolder)
	if err != nil {
		return false, nil
	}
	candidate := filepath.Join(canonicalDev, identity.Owner, identity.Repo)
	if !repoPathMatches(candidate, identity) {
		return false, nil
	}
	if err := locations.UpsertLocal(ctx, identity.Provider, identity.Owner, identity.Repo, candidate, true); err != nil {
		return false, fmt.Errorf("upsert targeted local repo %s: %w", identity.ID(), err)
	}
	return true, nil
}

var (
	errDevFolderPathRequired = errors.New("dev_folder_path is required")
	errDevFolderPathInvalid  = errors.New("dev_folder_path is invalid")
)

func destinationRoot(identity remote.Identity, destination, devFolder string, cacheRoot func() (string, error)) (string, string, error) {
	switch destination {
	case "dev_folder":
		if devFolder == "" {
			return "", "", errDevFolderPathRequired
		}
		canonical, err := scan.ValidateDevFolder(devFolder)
		if err != nil {
			return "", "", fmt.Errorf("%w: %w", errDevFolderPathInvalid, err)
		}
		return canonical, "local", nil
	case "cache":
		root, err := cacheRoot()
		if err != nil {
			return "", "", err
		}
		return root, "cache", nil
	default:
		return "", "", fmt.Errorf("unsupported destination %q for %s", destination, identity.ID())
	}
}

type destinationState struct {
	CanClone bool
	Matches  bool
	Message  string
}

func inspectDestination(path string, identity remote.Identity) (destinationState, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return destinationState{CanClone: true}, nil
	}
	if err != nil {
		return destinationState{}, err
	}
	if !info.IsDir() {
		return destinationState{Message: fmt.Sprintf("destination %q is not a directory", path)}, nil
	}
	if repoPathMatches(path, identity) {
		return destinationState{Matches: true}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return destinationState{}, err
	}
	if len(entries) > 0 {
		return destinationState{Message: fmt.Sprintf("destination %q is non-empty and does not match %s", path, identity.ID())}, nil
	}
	return destinationState{CanClone: true}, nil
}

func prepareCloneDestination(path string) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return false, fmt.Errorf("create clone destination %q: %w", path, err)
		}
		return true, nil
	} else if err != nil {
		return false, err
	}
	return false, nil
}

func cleanupBistroCreated(path string, createdByBistro bool, source, root string) error {
	if !createdByBistro {
		return nil
	}
	if source == "local" && !hasOnlyGitCloneFiles(path) {
		return fmt.Errorf("partial clone contains non-Bistro files; manual cleanup required: %s", path)
	}
	if source == "cache" && !isPathInside(root, path) {
		return fmt.Errorf("partial cache clone is outside cache root; manual cleanup required: %s", path)
	}
	return os.RemoveAll(path)
}

func validateCacheRepoPathInsideRoot(path string) error {
	root, err := clonecache.Root()
	if err != nil {
		return fmt.Errorf("get cache root: %w", err)
	}
	if !isPathInside(root, path) {
		return fmt.Errorf("cache path %q is outside cache root", path)
	}
	return nil
}

func isPathInside(root, path string) bool {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(canonicalRoot), filepath.Clean(canonicalPath))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func hasOnlyGitCloneFiles(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() != ".git" {
			return false
		}
	}
	return true
}

func setRepoSource(ctx context.Context, db *sql.DB, locations *repolocation.Store, identity remote.Identity, path, source string) error {
	if source == "cache" {
		if err := ensureRepoRow(ctx, db, identity); err != nil {
			return err
		}
		return locations.SetCache(ctx, identity.ID(), path)
	}
	return locations.UpsertLocal(ctx, identity.Provider, identity.Owner, identity.Repo, path, true)
}

type repoCloneDestination struct {
	URL, Path                       string
	Identity                        remote.Identity
	Source, Root                    string
	CreatedByBistro                 bool
	FailureLog                      string
	MismatchReason, MismatchMessage string
}

func (s *RepoLocation) cloneAndVerify(ctx context.Context, destination repoCloneDestination) error {
	logger := serviceLogger(s.Log)
	cleanupMessage := func(message string) string {
		if err := cleanupBistroCreated(destination.Path, destination.CreatedByBistro, destination.Source, destination.Root); err != nil {
			logger.Warn("clone cleanup failed", "error", err)
			return message + "; manual cleanup may be required"
		}
		return message
	}
	if err := cloneRepo(ctx, s.Clone, destination.URL, destination.Path, destination.Source == "cache"); err != nil {
		logger.Warn(destination.FailureLog, "error", err)
		message := cleanupMessage("unable to clone repository")
		if errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return locationError(cloneErrorCode(err), message)
	}
	if !repoPathMatches(destination.Path, destination.Identity) {
		return locationError(destination.MismatchReason, cleanupMessage(destination.MismatchMessage))
	}
	return nil
}

// cloneRepo delegates to the injected clone dependency; auth comes from the
// shared git client's token source configured in app.go.
func cloneRepo(ctx context.Context, clone func(context.Context, string, string, bool) error, repoURL, destination string, bare bool) error {
	if clone == nil {
		return errors.New("clone dependency is not configured")
	}
	return clone(ctx, repoURL, destination, bare)
}

func cloneErrorCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return locationCancelled
	}
	if git.IsAuthError(err) {
		return locationAuthFailed
	}
	return locationCloneFailed
}

func ensureRepoRow(ctx context.Context, db *sql.DB, identity remote.Identity) error {
	_, err := db.ExecContext(ctx, `INSERT INTO repos (id, provider, owner, repo, watched) VALUES (?, ?, ?, ?, 1) ON CONFLICT(id) DO NOTHING`, identity.ID(), identity.Provider, strings.ToLower(identity.Owner), strings.ToLower(identity.Repo))
	if err != nil {
		return fmt.Errorf("ensure repo row %s: %w", identity.ID(), err)
	}
	return nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
