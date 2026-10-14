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

const githubRepoURLFormat = "https://github.com/%s/%s.git"

// LocationErrorPayload is the structured error envelope consumed by Electron.
type LocationErrorPayload struct {
	OK        bool   `json:"ok"`
	RepoID    string `json:"repo_id"`
	Operation string `json:"operation"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

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

// NewValidateDevFolderHandler checks whether a development folder can be scanned.
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
			repoLocationLogger(ctx, log, msg).Warn("validate dev folder failed", "error", err)
			return locationResponse(msg, locationError("", operationValidateDevFolder, "invalid_path", "invalid development folder path")), nil
		}
		return locationResponse(msg, struct {
			OK            bool   `json:"ok"`
			CanonicalPath string `json:"canonical_path"`
		}{OK: true, CanonicalPath: canonicalPath}), nil
	}
}

// NewScanDevFolderHandler discovers checkouts and updates their location status.
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
			return locationResponse(msg, scanDevFolderPayload{OK: true}), nil
		}
		canonicalPath, err := scan.ValidateDevFolder(data.Path)
		if err != nil {
			repoLocationLogger(ctx, log, msg).Warn("scan dev folder validation failed", "error", err)
			return locationResponse(msg, locationError("", operationScanDevFolder, "invalid_path", "invalid development folder path")), nil
		}
		result, err := scan.Scan(ctx, canonicalPath)
		if err != nil {
			return nil, fmt.Errorf("scan dev folder %q: %w", canonicalPath, err)
		}
		healthy, degraded, err := updateScannedRepoLocations(ctx, locations, result.Found)
		if err != nil {
			return nil, err
		}
		triggerSync(syncTrigger, logger)
		return locationResponse(msg, scanDevFolderPayload{OK: true, Discovered: len(result.Found), Healthy: healthy, Degraded: degraded, Skipped: result.Skipped, Partial: result.Partial}), nil
	}
}

func updateScannedRepoLocations(ctx context.Context, locations *store.RepoLocationStore, found []scan.Found) (int, int, error) {
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
		identity, err := git.ParseRemoteURL(data.URL)
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
		matches, err := repoPathRemoteMatches(canonical, git.Identity{Provider: location.Provider, Owner: location.Owner, Repo: location.Repo})
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
		identity := git.Identity{Provider: location.Provider, Owner: location.Owner, Repo: location.Repo}
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

func locationResponse(msg ws.InboundMessage, data any) map[string]any {
	return map[string]any{"id": msg.ID, "type": msg.Type + "_RESULT", "data": data}
}

func locationError(repoID, operation, code, message string) LocationErrorPayload {
	return LocationErrorPayload{OK: false, RepoID: repoID, Operation: operation, Code: code, Message: message}
}

type scanDevFolderPayload struct {
	OK         bool `json:"ok"`
	Discovered int  `json:"discovered"`
	Healthy    int  `json:"healthy"`
	Degraded   int  `json:"degraded"`
	Skipped    int  `json:"skipped"`
	Partial    bool `json:"partial"`
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

func healthCheckLocalRepo(ctx context.Context, row store.RepoLocation) (string, string, string) {
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
	if err := locations.UpsertLocal(ctx, identity.Provider, identity.Owner, identity.Repo, candidate, true); err != nil {
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
	return locations.UpsertLocal(ctx, identity.Provider, identity.Owner, identity.Repo, path, true)
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
