package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/itterno/bistro/internal/git/remote"
)

type AddRepoByURLRequest struct {
	URL           string  `json:"url"`
	Destination   *string `json:"destination"`
	DevFolderPath *string `json:"dev_folder_path"`
}
type AddRepoResult struct {
	RepoID string `json:"repo_id"`
	Source string `json:"source"`
}
type RecoverPathRequest struct {
	RepoID string `json:"repo_id"`
	Path   string `json:"path"`
}

// AddByURL links an existing checkout or clones the requested repository.
func (s *RepoLocation) AddByURL(ctx context.Context, req AddRepoByURLRequest) (AddRepoResult, error) {
	return guard(s.Panics, "RepoLocation.AddByURL", func() (AddRepoResult, error) {
		identity, err := remote.ParseURL(req.URL)
		if err != nil {
			serviceLogger(s.Log).Warn("parse repository URL failed", "error", err)
			return AddRepoResult{}, locationError("invalid_url", "invalid repository URL")
		}
		repoID := identity.ID()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return AddRepoResult{}, ctxErr
		}
		isRepoDirOverrideSet := isBistroRepoDirOverrideSet()
		if !isRepoDirOverrideSet {
			if found, locationErr, err := registeredLocal(ctx, s.Store, repoID); err != nil {
				return AddRepoResult{}, err
			} else if locationErr != nil {
				return AddRepoResult{}, locationErr
			} else if found {
				return AddRepoResult{RepoID: repoID, Source: "local"}, nil
			}
		}
		if !isRepoDirOverrideSet && req.DevFolderPath != nil && *req.DevFolderPath != "" {
			if found, err := targetedDevFolder(ctx, s.Store, identity, *req.DevFolderPath); err != nil {
				return AddRepoResult{}, err
			} else if found {
				return AddRepoResult{RepoID: repoID, Source: "local"}, nil
			}
		}
		destination := "cache"
		if !isRepoDirOverrideSet {
			if req.Destination == nil || *req.Destination == "" {
				return AddRepoResult{}, locationError("destination_required", "destination is required")
			}
			destination = *req.Destination
		}
		rootPath, source, err := destinationRoot(identity, destination, valueOrEmpty(req.DevFolderPath), s.CacheRoot)
		if err != nil {
			serviceLogger(s.Log).Warn("resolve repo destination failed", "error", err)
			if ctxErr := contextError(err); ctxErr != nil {
				return AddRepoResult{}, ctxErr
			}
			return AddRepoResult{}, locationError(destinationRootErrorCode(err), "invalid repository destination")
		}
		destinationPath := filepath.Join(rootPath, identity.Owner, identity.Repo)
		state, err := inspectDestination(destinationPath, identity)
		if err != nil {
			serviceLogger(s.Log).Warn("inspect repo destination failed", "error", err)
			if ctxErr := contextError(err); ctxErr != nil {
				return AddRepoResult{}, ctxErr
			}
			return AddRepoResult{}, locationError("destination_conflict", "unable to inspect destination")
		}
		if state.Matches {
			if err := setRepoSource(ctx, s.DB, s.Store, identity, destinationPath, source); err != nil {
				return AddRepoResult{}, err
			}
			return AddRepoResult{RepoID: repoID, Source: source}, nil
		}
		if !state.CanClone {
			return AddRepoResult{}, locationError("destination_conflict", state.Message)
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return AddRepoResult{}, fmt.Errorf("create destination parent: %w", err)
		}
		createdByBistro, err := prepareCloneDestination(destinationPath)
		if err != nil {
			return AddRepoResult{}, err
		}
		if err := cloneRepo(ctx, s.Clone, req.URL, destinationPath, source == "cache"); err != nil {
			logger := serviceLogger(s.Log)
			logger.Warn("clone repository failed", "error", err)
			message := "unable to clone repository"
			if cleanupErr := cleanupBistroCreated(destinationPath, createdByBistro, source, rootPath); cleanupErr != nil {
				logger.Warn("clone cleanup failed", "error", cleanupErr)
				message += "; manual cleanup may be required"
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return AddRepoResult{}, err
			}
			return AddRepoResult{}, locationError(cloneErrorCode(err), message)
		}
		if !repoPathMatches(destinationPath, identity) {
			message := "cloned repository remote does not match requested URL"
			if cleanupErr := cleanupBistroCreated(destinationPath, createdByBistro, source, rootPath); cleanupErr != nil {
				serviceLogger(s.Log).Warn("clone cleanup failed", "error", cleanupErr)
				message += "; manual cleanup may be required"
			}
			return AddRepoResult{}, locationError("remote_mismatch", message)
		}
		if err := setRepoSource(ctx, s.DB, s.Store, identity, destinationPath, source); err != nil {
			return AddRepoResult{}, err
		}
		return AddRepoResult{RepoID: repoID, Source: source}, nil
	})
}
