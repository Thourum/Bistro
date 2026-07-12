package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"

	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

const (
	operationValidateDevFolder  = "validate_dev_folder"
	operationScanDevFolder      = "scan_dev_folder"
	operationAddRepoByURL       = "add_repo_by_url"
	operationPruneCache         = "prune_cache"
	operationClearCache         = "clear_cache"
	operationDeleteCacheRepo    = "delete_cache_repo"
	operationRecoverRepoPath    = "recover_repo_path"
	operationRecoverRepoToCache = "recover_repo_to_cache"
)

// LocationErrorPayload is the structured error envelope consumed by Electron.
type LocationErrorPayload struct {
	OK        bool   `json:"ok"`
	RepoID    string `json:"repo_id"`
	Operation string `json:"operation"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

type AddRepoByURLDeps struct {
	DB        *sql.DB
	Store     *store.RepoLocationStore
	Clone     func(ctx context.Context, repoURL, destination string, bare bool) error
	CacheRoot func() (string, error)
	Log       *slog.Logger
}

type RemoveRepoDeps struct {
	DB             *sql.DB
	Store          *store.RepoLocationStore
	Locks          *git.RepoLocks
	DeleteCacheDir func(path string) error
	Log            *slog.Logger
}

type CacheMaintenanceDeps struct {
	Store          *store.RepoLocationStore
	Locks          *git.RepoLocks
	DeleteCacheDir func(path string) error
	SweepOrphans   func(known map[string]bool) ([]string, error)
	Log            *slog.Logger
}

type RecoverRepoToCacheDeps struct {
	DB        *sql.DB
	Store     *store.RepoLocationStore
	Clone     func(ctx context.Context, repoURL, destination string, bare bool) error
	CacheRoot func() (string, error)
	Log       *slog.Logger
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

type removeCounts struct {
	PRs       int `json:"prs"`
	Comments  int `json:"comments"`
	Threads   int `json:"threads"`
	Patchsets int `json:"patchsets"`
	Actions   int `json:"actions"`
}

func NewValidateDevFolderHandler(log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var data struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid VALIDATE_DEV_FOLDER payload: %w", err)
		}
		canonicalPath, err := scan.ValidateDevFolder(data.Path)
		if err != nil {
			return locationResponse(msg, locationError("", operationValidateDevFolder, "invalid_path", err.Error())), nil
		}
		return locationResponse(msg, struct {
			OK            bool   `json:"ok"`
			CanonicalPath string `json:"canonical_path"`
		}{OK: true, CanonicalPath: canonicalPath}), nil
	}
}

func NewScanDevFolderHandler(locations *store.RepoLocationStore, syncTrigger chan<- string, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := repoLocationLogger(ctx, log, msg)
		var data struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return nil, fmt.Errorf("invalid SCAN_DEV_FOLDER payload: %w", err)
		}
		if isBistroRepoDirOverrideSet() {
			return locationResponse(msg, scanDevFolderPayload(true, 0, 0, 0, 0, false)), nil
		}
		canonicalPath, err := scan.ValidateDevFolder(data.Path)
		if err != nil {
			return locationResponse(msg, locationError("", operationScanDevFolder, "invalid_path", err.Error())), nil
		}
		result, err := scan.Scan(ctx, canonicalPath)
		if err != nil {
			return nil, fmt.Errorf("scan dev folder %q: %w", canonicalPath, err)
		}
		for _, found := range result.Found {
			if err := locations.UpsertLocal(ctx, found.Identity.Provider, found.Identity.Owner, found.Identity.Repo, found.Path); err != nil {
				return nil, fmt.Errorf("upsert scanned repo %s: %w", found.Identity.ID(), err)
			}
		}
		localRows, err := locations.ListBySource(ctx, "local")
		if err != nil {
			return nil, fmt.Errorf("list local repo locations: %w", err)
		}
		healthy := 0
		degraded := 0
		for _, row := range localRows {
			status, code, message := healthCheckLocalRepo(row)
			if status == "ok" {
				if err := locations.ClearDegraded(ctx, row.RepoID); err != nil {
					return nil, fmt.Errorf("clear degraded %s: %w", row.RepoID, err)
				}
				healthy++
				continue
			}
			if err := locations.MarkDegraded(ctx, row.RepoID, status, code, message); err != nil {
				return nil, fmt.Errorf("mark degraded %s: %w", row.RepoID, err)
			}
			degraded++
		}
		triggerSync(syncTrigger, logger)
		return locationResponse(msg, scanDevFolderPayload(true, len(result.Found), healthy, degraded, result.Skipped, result.Partial)), nil
	}
}

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
		identity, err := git.ParseRemoteURL(data.URL)
		if err != nil {
			return locationResponse(msg, locationError("", operationAddRepoByURL, "invalid_url", err.Error())), nil
		}
		repoID := identity.ID()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, cloneErrorCode(ctxErr), ctxErr.Error())), nil
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
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, destinationRootErrorCode(err), err.Error())), nil
		}
		destinationPath := filepath.Join(rootPath, identity.Owner, identity.Repo)
		state, err := inspectDestination(destinationPath, identity)
		if err != nil {
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, "destination_conflict", err.Error())), nil
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
			message := err.Error()
			if cleanupErr := cleanupBistroCreated(destinationPath, createdByBistro, source, rootPath); cleanupErr != nil {
				message = fmt.Sprintf("%s; %s", message, cleanupErr.Error())
			}
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, cloneErrorCode(err), message)), nil
		}
		if !repoPathMatches(destinationPath, identity) {
			message := "cloned repository remote does not match requested URL"
			if cleanupErr := cleanupBistroCreated(destinationPath, createdByBistro, source, rootPath); cleanupErr != nil {
				message = fmt.Sprintf("%s; %s", message, cleanupErr.Error())
			}
			return locationResponse(msg, locationError(repoID, operationAddRepoByURL, "remote_mismatch", message)), nil
		}
		if err := setRepoSource(ctx, deps.DB, deps.Store, identity, destinationPath, source); err != nil {
			return nil, err
		}
		return locationResponse(msg, repoSourcePayload(repoID, source)), nil
	}
}

func NewRemoveRepoPreflightHandler(db *sql.DB, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		repoID, err := repoIDFromMessage(msg, "REMOVE_REPO_PREFLIGHT")
		if err != nil {
			return nil, err
		}
		counts, err := countRepoRows(ctx, db, repoID)
		if err != nil {
			return nil, err
		}
		return locationResponse(msg, struct {
			OK     bool         `json:"ok"`
			Counts removeCounts `json:"counts"`
		}{OK: true, Counts: counts}), nil
	}
}

func NewRemoveRepoHandler(deps RemoveRepoDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		repoID, err := repoIDFromMessage(msg, "REMOVE_REPO")
		if err != nil {
			return nil, err
		}
		// Read location, delete rows, and delete the cache dir under the repo
		// lock so a concurrent clone/recovery cannot race us into deleting a
		// stale path.
		var counts removeCounts
		operation := func() error {
			location, err := deps.Store.Get(ctx, repoID)
			if err != nil {
				return err
			}
			counts, err = removeRepoTx(ctx, deps.DB, repoID)
			if err != nil {
				return err
			}
			if location != nil && location.Source == "cache" && location.LocalPath != "" && deps.DeleteCacheDir != nil {
				if err := deps.DeleteCacheDir(location.LocalPath); err != nil && deps.Log != nil {
					deps.Log.Warn("delete cache dir after remove repo failed", "repo_id", repoID, "error", err)
				}
			}
			return nil
		}
		if deps.Locks != nil {
			err = deps.Locks.WithRepo(repoID, operation)
		} else {
			err = operation()
		}
		if err != nil {
			return nil, err
		}
		return locationResponse(msg, struct {
			OK      bool         `json:"ok"`
			Deleted removeCounts `json:"deleted"`
		}{OK: true, Deleted: counts}), nil
	}
}

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

func NewPruneCacheHandler(deps CacheMaintenanceDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var payload any
		operation := func() error {
			removed, failed, err := pruneCache(ctx, deps.Store, requireDeleteCacheDir(deps.DeleteCacheDir))
			if err != nil {
				return err
			}
			payload = cacheMutationPayload(true, removed, failed, 0)
			return nil
		}
		if err := runCacheMaintenance(deps.Locks, operation); err != nil {
			return locationResponse(msg, locationError("", operationPruneCache, "delete_failed", err.Error())), nil
		}
		return locationResponse(msg, payload), nil
	}
}

func NewClearCacheHandler(deps CacheMaintenanceDeps) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		var payload any
		operation := func() error {
			removed, failed, orphansRemoved, err := clearCache(ctx, deps.Store, requireDeleteCacheDir(deps.DeleteCacheDir), deps.SweepOrphans)
			if err != nil {
				return err
			}
			payload = cacheMutationPayload(true, removed, failed, orphansRemoved)
			return nil
		}
		if err := runCacheMaintenance(deps.Locks, operation); err != nil {
			return locationResponse(msg, locationError("", operationClearCache, "delete_failed", err.Error())), nil
		}
		return locationResponse(msg, payload), nil
	}
}

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
				return locationResponse(msg, locationError(repoID, operationDeleteCacheRepo, "delete_failed", err.Error())), nil
			}
		} else if err := operation(); err != nil {
			return locationResponse(msg, locationError(repoID, operationDeleteCacheRepo, "delete_failed", err.Error())), nil
		}
		return locationResponse(msg, struct {
			OK bool `json:"ok"`
		}{OK: true}), nil
	}
}

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
			return locationResponse(msg, locationError(data.RepoID, operationRecoverRepoPath, recoverPathErrorCode(err), err.Error())), nil
		}
		matches, err := repoPathRemoteMatches(canonical, git.Identity{Provider: location.Provider, Owner: location.Owner, Repo: location.Repo})
		if err != nil {
			return locationResponse(msg, locationError(data.RepoID, operationRecoverRepoPath, recoverPathErrorCode(err), err.Error())), nil
		}
		if !matches {
			return locationResponse(msg, locationError(data.RepoID, operationRecoverRepoPath, "remote_mismatch", "path remote does not match repo")), nil
		}
		if err := locations.UpsertLocal(ctx, location.Provider, location.Owner, location.Repo, canonical); err != nil {
			return nil, err
		}
		return locationResponse(msg, struct {
			OK bool `json:"ok"`
		}{OK: true}), nil
	}
}

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
		identity := git.Identity{Provider: location.Provider, Owner: location.Owner, Repo: location.Repo}
		root, err := deps.CacheRoot()
		if err != nil {
			return nil, err
		}
		destination := filepath.Join(root, identity.Owner, identity.Repo)
		state, err := inspectDestination(destination, identity)
		if err != nil {
			return locationResponse(msg, locationError(repoID, operationRecoverRepoToCache, "clone_failed", err.Error())), nil
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
		repoURL := fmt.Sprintf("https://github.com/%s/%s.git", identity.Owner, identity.Repo)
		if err := cloneRepo(ctx, deps.Clone, repoURL, destination, true); err != nil {
			message := err.Error()
			if cleanupErr := cleanupBistroCreated(destination, createdByBistro, "cache", root); cleanupErr != nil {
				message = fmt.Sprintf("%s; %s", message, cleanupErr.Error())
			}
			return locationResponse(msg, locationError(repoID, operationRecoverRepoToCache, cloneErrorCode(err), message)), nil
		}
		if !repoPathMatches(destination, identity) {
			message := "cloned repository remote does not match repo"
			if cleanupErr := cleanupBistroCreated(destination, createdByBistro, "cache", root); cleanupErr != nil {
				message = fmt.Sprintf("%s; %s", message, cleanupErr.Error())
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

func locationResponse(msg ws.InboundMessage, data any) map[string]any {
	return map[string]any{"id": msg.ID, "type": msg.Type + "_RESULT", "data": data}
}

func locationError(repoID, operation, code, message string) LocationErrorPayload {
	return LocationErrorPayload{OK: false, RepoID: repoID, Operation: operation, Code: code, Message: message}
}

func scanDevFolderPayload(ok bool, discovered, healthy, degraded, skipped int, partial bool) struct {
	OK         bool `json:"ok"`
	Discovered int  `json:"discovered"`
	Healthy    int  `json:"healthy"`
	Degraded   int  `json:"degraded"`
	Skipped    int  `json:"skipped"`
	Partial    bool `json:"partial"`
} {
	return struct {
		OK         bool `json:"ok"`
		Discovered int  `json:"discovered"`
		Healthy    int  `json:"healthy"`
		Degraded   int  `json:"degraded"`
		Skipped    int  `json:"skipped"`
		Partial    bool `json:"partial"`
	}{OK: ok, Discovered: discovered, Healthy: healthy, Degraded: degraded, Skipped: skipped, Partial: partial}
}

func repoSourcePayload(repoID, source string) struct {
	OK     bool   `json:"ok"`
	RepoID string `json:"repo_id"`
	Source string `json:"source"`
} {
	return struct {
		OK     bool   `json:"ok"`
		RepoID string `json:"repo_id"`
		Source string `json:"source"`
	}{OK: true, RepoID: repoID, Source: source}
}

func repoLocationLogger(ctx context.Context, log *slog.Logger, msg ws.InboundMessage) *slog.Logger {
	logger := logging.FromContext(ctx)
	if log != nil {
		logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
	}
	return logger
}

func runCacheMaintenance(locks *git.RepoLocks, operation func() error) error {
	if locks != nil {
		return locks.WithMaintenance(operation)
	}
	return operation()
}

func isBistroRepoDirOverrideSet() bool {
	return os.Getenv("BISTRO_REPO_DIR") != ""
}

func triggerSync(syncTrigger chan<- string, logger *slog.Logger) {
	if syncTrigger == nil {
		return
	}
	select {
	case syncTrigger <- "repo_locations":
	default:
		if logger != nil {
			logger.Warn("sync trigger channel full, skipping repo location sync trigger")
		}
	}
}

func healthCheckLocalRepo(row store.RepoLocation) (string, string, string) {
	info, err := os.Stat(row.LocalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing", "path_missing", err.Error()
		}
		if os.IsPermission(err) {
			return "permission_denied", "permission_denied", err.Error()
		}
		return "missing", "path_missing", err.Error()
	}
	if !info.IsDir() {
		return "missing", "path_missing", "path is not a directory"
	}
	if ok, err := canReadDir(row.LocalPath); !ok {
		return "permission_denied", "permission_denied", err.Error()
	}
	identity := git.Identity{Provider: row.Provider, Owner: row.Owner, Repo: row.Repo}
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

func repoPathMatches(path string, identity git.Identity) bool {
	matches, err := repoPathRemoteMatches(path, identity)
	return err == nil && matches
}

func repoPathRemoteMatches(path string, identity git.Identity) (bool, error) {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return false, fmt.Errorf("open git repo %q: %w", path, err)
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return false, fmt.Errorf("read remotes for %q: %w", path, err)
	}
	for _, remote := range remotes {
		for _, rawURL := range remote.Config().URLs {
			remoteID, err := git.ParseRemoteURL(rawURL)
			if err == nil && remoteID.ID() == identity.ID() {
				return true, nil
			}
		}
	}
	return false, nil
}

func destinationRootErrorCode(err error) string {
	if errors.Is(err, errDevFolderPathRequired) {
		return "destination_required"
	}
	if errors.Is(err, errDevFolderPathInvalid) {
		return "invalid_path"
	}
	return "destination_conflict"
}

func recoverPathErrorCode(err error) string {
	if os.IsPermission(err) || errors.Is(err, os.ErrPermission) {
		return "permission_denied"
	}
	return "path_missing"
}

func registeredLocal(ctx context.Context, locations *store.RepoLocationStore, repoID string) (bool, *LocationErrorPayload, error) {
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
		code = "path_missing"
	}
	message := location.LocationError
	if message == "" {
		message = fmt.Sprintf("local repository is degraded: %s", location.LocationStatus)
	}
	payload := locationError(repoID, operationAddRepoByURL, code, message)
	return false, &payload, nil
}

func targetedDevFolder(ctx context.Context, locations *store.RepoLocationStore, identity git.Identity, devFolder string) (bool, error) {
	canonicalDev, err := filepath.EvalSymlinks(devFolder)
	if err != nil {
		return false, nil
	}
	candidate := filepath.Join(canonicalDev, identity.Owner, identity.Repo)
	if !repoPathMatches(candidate, identity) {
		return false, nil
	}
	if err := locations.UpsertLocal(ctx, identity.Provider, identity.Owner, identity.Repo, candidate); err != nil {
		return false, fmt.Errorf("upsert targeted local repo %s: %w", identity.ID(), err)
	}
	return true, nil
}

var (
	errDevFolderPathRequired = errors.New("dev_folder_path is required")
	errDevFolderPathInvalid  = errors.New("dev_folder_path is invalid")
)

func destinationRoot(identity git.Identity, destination, devFolder string, cacheRoot func() (string, error)) (string, string, error) {
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

func inspectDestination(path string, identity git.Identity) (destinationState, error) {
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
	root, err := git.CacheRoot()
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

func setRepoSource(ctx context.Context, db *sql.DB, locations *store.RepoLocationStore, identity git.Identity, path, source string) error {
	if source == "cache" {
		if err := ensureRepoRow(ctx, db, identity); err != nil {
			return err
		}
		return locations.SetCache(ctx, identity.ID(), path)
	}
	return locations.UpsertLocal(ctx, identity.Provider, identity.Owner, identity.Repo, path)
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
		return "cancelled"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "auth") || strings.Contains(message, "authentication") || strings.Contains(message, "authorization") {
		return "auth_failed"
	}
	return "clone_failed"
}

func ensureRepoRow(ctx context.Context, db *sql.DB, identity git.Identity) error {
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

func repoIDFromMessage(msg ws.InboundMessage, messageType string) (string, error) {
	var data struct {
		RepoID string `json:"repo_id"`
	}
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return "", fmt.Errorf("invalid %s payload: %w", messageType, err)
	}
	if data.RepoID == "" {
		return "", fmt.Errorf("%s: repo_id is required", messageType)
	}
	return data.RepoID, nil
}

func countRepoRows(ctx context.Context, db *sql.DB, repoID string) (removeCounts, error) {
	var counts removeCounts
	queries := []struct {
		dest  *int
		query string
	}{
		{&counts.PRs, `SELECT COUNT(*) FROM prs WHERE repo_id = ?`},
		{&counts.Comments, `SELECT COUNT(*) FROM comments WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Threads, `SELECT COUNT(*) FROM review_threads WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Patchsets, `SELECT COUNT(*) FROM patchsets WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Actions, `SELECT COUNT(*) FROM action_queue WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
	}
	for _, query := range queries {
		if err := db.QueryRowContext(ctx, query.query, repoID).Scan(query.dest); err != nil {
			return removeCounts{}, fmt.Errorf("count repo rows %s: %w", repoID, err)
		}
	}
	return counts, nil
}

func removeRepoTx(ctx context.Context, db *sql.DB, repoID string) (removeCounts, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return removeCounts{}, fmt.Errorf("begin remove repo tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	counts, err := countRepoRowsTx(ctx, tx, repoID)
	if err != nil {
		return removeCounts{}, err
	}
	prSubquery := `SELECT id FROM prs WHERE repo_id = ?`
	deleteStatements := []string{
		`DELETE FROM action_queue WHERE pr_id IN (` + prSubquery + `)`,
		`DELETE FROM patchsets WHERE pr_id IN (` + prSubquery + `)`,
		`DELETE FROM last_reviewed_sha WHERE pr_id IN (` + prSubquery + `)`,
		`DELETE FROM files_viewed WHERE pr_id IN (` + prSubquery + `)`,
		`DELETE FROM prs WHERE repo_id = ?`,
		`DELETE FROM repos WHERE id = ?`,
	}
	for _, statement := range deleteStatements {
		if _, err := tx.ExecContext(ctx, statement, repoID); err != nil {
			return removeCounts{}, fmt.Errorf("remove repo %s: %w", repoID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return removeCounts{}, fmt.Errorf("commit remove repo %s: %w", repoID, err)
	}
	committed = true
	return counts, nil
}

func countRepoRowsTx(ctx context.Context, tx *sql.Tx, repoID string) (removeCounts, error) {
	var counts removeCounts
	queries := []struct {
		dest  *int
		query string
	}{
		{&counts.PRs, `SELECT COUNT(*) FROM prs WHERE repo_id = ?`},
		{&counts.Comments, `SELECT COUNT(*) FROM comments WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Threads, `SELECT COUNT(*) FROM review_threads WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Patchsets, `SELECT COUNT(*) FROM patchsets WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
		{&counts.Actions, `SELECT COUNT(*) FROM action_queue WHERE pr_id IN (SELECT id FROM prs WHERE repo_id = ?)`},
	}
	for _, query := range queries {
		if err := tx.QueryRowContext(ctx, query.query, repoID).Scan(query.dest); err != nil {
			return removeCounts{}, fmt.Errorf("count repo rows %s: %w", repoID, err)
		}
	}
	return counts, nil
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
			failed = append(failed, cacheFailurePayload{RepoID: stat.RepoID, Code: "delete_failed", Message: err.Error()})
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
			failed = append(failed, cacheFailurePayload{RepoID: row.RepoID, Code: "delete_failed", Message: err.Error()})
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
			failed = append(failed, cacheFailurePayload{Code: "delete_failed", Message: fmt.Sprintf("sweep orphans: %s", err)})
		}
	}
	return removed, failed, orphansRemoved, nil
}

func cacheMutationPayload(ok bool, removed []string, failed []cacheFailurePayload, orphansRemoved int) struct {
	OK             bool                  `json:"ok"`
	Removed        []string              `json:"removed"`
	Failed         []cacheFailurePayload `json:"failed"`
	OrphansRemoved int                   `json:"orphans_removed"`
} {
	return struct {
		OK             bool                  `json:"ok"`
		Removed        []string              `json:"removed"`
		Failed         []cacheFailurePayload `json:"failed"`
		OrphansRemoved int                   `json:"orphans_removed"`
	}{OK: ok, Removed: removed, Failed: failed, OrphansRemoved: orphansRemoved}
}
