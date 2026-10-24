package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"

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
	case syncTrigger <- "":
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
