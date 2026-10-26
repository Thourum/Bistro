package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

// AddRepoByURLDeps supplies repository persistence, cloning, and logging.
type AddRepoByURLDeps struct {
	DB        *sql.DB
	Store     *store.RepoLocationStore
	Clone     func(ctx context.Context, repoURL, destination string, bare bool) error
	CacheRoot func() (string, error)
	Log       *slog.Logger
}

// RecoverRepoToCacheDeps supplies repository persistence and cache cloning.
type RecoverRepoToCacheDeps struct {
	DB        *sql.DB
	Store     *store.RepoLocationStore
	Clone     func(ctx context.Context, repoURL, destination string, bare bool) error
	CacheRoot func() (string, error)
	Log       *slog.Logger
}

// NewAddRepoByURLHandler links an existing checkout or clones the requested repository.
func NewAddRepoByURLHandler(deps AddRepoByURLDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var data struct {
			URL           string  `json:"url"`
			Destination   *string `json:"destination"`
			DevFolderPath *string `json:"dev_folder_path"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid ADD_REPO_BY_URL payload: %w", err)
		}
		identity, err := remote.ParseURL(data.URL)
		if err != nil {
			repoLocationLogger(ctx, deps.Log, msg).Warn("parse repository URL failed", "error", err)
			return locationResponse(msg, locationError("", operationAddRepoByURL, "invalid_url", "invalid repository URL")), nil
		}
		repoID := identity.ID()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, cloneErrorCode(ctxErr), "repository add cancelled")), nil
		}
		isRepoDirOverrideSet := isBistroRepoDirOverrideSet()
		if !isRepoDirOverrideSet {
			if found, locationErr, err := registeredLocal(ctx, deps.Store, repoID); err != nil {
				return nil, err
			} else if locationErr != nil {
				return locationResponse(msg, *locationErr), nil
			} else if found {
				return locationResponse(msg, repoSourcePayload(repoID, "local")), nil
			}
		}
		if !isRepoDirOverrideSet && data.DevFolderPath != nil && *data.DevFolderPath != "" {
			if found, err := targetedDevFolder(ctx, deps.Store, identity, *data.DevFolderPath); err != nil {
				return nil, err
			} else if found {
				return locationResponse(msg, repoSourcePayload(repoID, "local")), nil
			}
		}
		destination := "cache"
		if !isRepoDirOverrideSet {
			if data.Destination == nil || *data.Destination == "" {
				return locationResponse(msg, locationError(repoID, operationAddRepoByURL, "destination_required", "destination is required")), nil
			}
			destination = *data.Destination
		}
		rootPath, source, err := destinationRoot(identity, destination, valueOrEmpty(data.DevFolderPath), deps.CacheRoot)
		if err != nil {
			repoLocationLogger(ctx, deps.Log, msg).Warn("resolve repo destination failed", "error", err)
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, destinationRootErrorCode(err), "invalid repository destination")), nil
		}
		destinationPath := filepath.Join(rootPath, identity.Owner, identity.Repo)
		state, err := inspectDestination(destinationPath, identity)
		if err != nil {
			repoLocationLogger(ctx, deps.Log, msg).Warn("inspect repo destination failed", "error", err)
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, "destination_conflict", "unable to inspect destination")), nil
		}
		if state.Matches {
			if err := setRepoSource(ctx, deps.DB, deps.Store, identity, destinationPath, source); err != nil {
				return nil, err
			}
			return locationResponse(msg, repoSourcePayload(repoID, source)), nil
		}
		if !state.CanClone {
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, "destination_conflict", state.Message)), nil
		}
		if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
			return nil, fmt.Errorf("create destination parent: %w", err)
		}
		createdByBistro, err := prepareCloneDestination(destinationPath)
		if err != nil {
			return nil, err
		}
		if err := cloneRepo(ctx, deps.Clone, data.URL, destinationPath, source == "cache"); err != nil {
			logger := repoLocationLogger(ctx, deps.Log, msg)
			logger.Warn("clone repository failed", "error", err)
			message := "unable to clone repository"
			if cleanupErr := cleanupBistroCreated(destinationPath, createdByBistro, source, rootPath); cleanupErr != nil {
				logger.Warn("clone cleanup failed", "error", cleanupErr)
				message += "; manual cleanup may be required"
			}
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, cloneErrorCode(err), message)), nil
		}
		if !repoPathMatches(destinationPath, identity) {
			message := "cloned repository remote does not match requested URL"
			if cleanupErr := cleanupBistroCreated(destinationPath, createdByBistro, source, rootPath); cleanupErr != nil {
				repoLocationLogger(ctx, deps.Log, msg).Warn("clone cleanup failed", "error", cleanupErr)
				message += "; manual cleanup may be required"
			}
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, "remote_mismatch", message)), nil
		}
		if err := setRepoSource(ctx, deps.DB, deps.Store, identity, destinationPath, source); err != nil {
			return nil, err
		}
		return locationResponse(msg, repoSourcePayload(repoID, source)), nil
	}
}

// NewRecoverRepoPathHandler re-links a repository to a matching local checkout.
func NewRecoverRepoPathHandler(locations *store.RepoLocationStore, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var data struct {
			RepoID string `json:"repo_id"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid RECOVER_REPO_PATH payload: %w", err)
		}
		location, err := locations.Get(ctx, data.RepoID)
		if err != nil {
			return nil, err
		}
		if location == nil {
			return locationResponse(msg, locationError(data.RepoID, operationRecoverRepoPath, "path_missing", "repo not found")), nil
		}
		canonical, err := filepath.EvalSymlinks(data.Path)
		if err != nil {
			repoLocationLogger(ctx, log, msg).Warn("recover repository path failed", "error", err)
			return locationResponse(msg, locationError(data.RepoID, operationRecoverRepoPath, recoverPathErrorCode(err), "unable to read repository path")), nil
		}
		matches, err := repoPathRemoteMatches(canonical, remote.Identity{Provider: location.Provider, Owner: location.Owner, Repo: location.Repo})
		if err != nil {
			repoLocationLogger(ctx, log, msg).Warn("recover repository path failed", "error", err)
			return locationResponse(msg, locationError(data.RepoID, operationRecoverRepoPath, recoverPathErrorCode(err), "unable to read repository path")), nil
		}
		if !matches {
			return locationResponse(msg, locationError(data.RepoID, operationRecoverRepoPath, "remote_mismatch", "path remote does not match repo")), nil
		}
		if err := locations.UpsertLocal(ctx, location.Provider, location.Owner, location.Repo, canonical, false); err != nil {
			return nil, err
		}
		return locationResponse(msg, struct {
			OK bool `json:"ok"`
		}{OK: true}), nil
	}
}

// NewRecoverRepoToCacheHandler clones a repository into the managed cache.
func NewRecoverRepoToCacheHandler(deps RecoverRepoToCacheDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		repoID, err := repoIDFromMessage(msg, "RECOVER_REPO_TO_CACHE")
		if err != nil {
			return nil, err
		}
		location, err := deps.Store.Get(ctx, repoID)
		if err != nil {
			return nil, err
		}
		if location == nil {
			return locationResponse(msg, locationError(repoID, operationRecoverRepoToCache, "clone_failed", "repo not found")), nil
		}
		identity := remote.Identity{Provider: location.Provider, Owner: location.Owner, Repo: location.Repo}
		root, err := deps.CacheRoot()
		if err != nil {
			return nil, err
		}
		destination := filepath.Join(root, identity.Owner, identity.Repo)
		state, err := inspectDestination(destination, identity)
		if err != nil {
			repoLocationLogger(ctx, deps.Log, msg).Warn("inspect cache destination failed", "error", err)
			return locationResponse(msg, locationError(repoID, operationRecoverRepoToCache, "clone_failed", "unable to prepare cache repository")), nil
		}
		if state.Matches {
			if err := deps.Store.SetCache(ctx, repoID, destination); err != nil {
				return nil, err
			}
			return locationResponse(msg, struct {
				OK bool `json:"ok"`
			}{OK: true}), nil
		}
		if !state.CanClone {
			return locationResponse(msg, locationError(repoID, operationRecoverRepoToCache, "clone_failed", state.Message)), nil
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return nil, err
		}
		createdByBistro, err := prepareCloneDestination(destination)
		if err != nil {
			return nil, err
		}
		repoURL := fmt.Sprintf(githubRepoURLFormat, identity.Owner, identity.Repo)
		if err := cloneRepo(ctx, deps.Clone, repoURL, destination, true); err != nil {
			logger := repoLocationLogger(ctx, deps.Log, msg)
			logger.Warn("recover cache clone failed", "error", err)
			message := "unable to clone repository"
			if cleanupErr := cleanupBistroCreated(destination, createdByBistro, "cache", root); cleanupErr != nil {
				logger.Warn("clone cleanup failed", "error", cleanupErr)
				message += "; manual cleanup may be required"
			}
			return locationResponse(msg, locationError(repoID, operationRecoverRepoToCache, cloneErrorCode(err), message)), nil
		}
		if !repoPathMatches(destination, identity) {
			message := "cloned repository remote does not match repo"
			if cleanupErr := cleanupBistroCreated(destination, createdByBistro, "cache", root); cleanupErr != nil {
				repoLocationLogger(ctx, deps.Log, msg).Warn("clone cleanup failed", "error", cleanupErr)
				message += "; manual cleanup may be required"
			}
			return locationResponse(msg, locationError(repoID, operationRecoverRepoToCache, "clone_failed", message)), nil
		}
		if err := deps.Store.SetCache(ctx, repoID, destination); err != nil {
			return nil, err
		}
		return locationResponse(msg, struct {
			OK bool `json:"ok"`
		}{OK: true}), nil
	}
}
