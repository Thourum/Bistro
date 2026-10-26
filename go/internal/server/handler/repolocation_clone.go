package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/itterno/bistro/internal/git/clonecache"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/scan"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

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

func targetedDevFolder(ctx context.Context, locations *store.RepoLocationStore, identity remote.Identity, devFolder string) (bool, error) {
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

func setRepoSource(ctx context.Context, db *sql.DB, locations *store.RepoLocationStore, identity remote.Identity, path, source string) error {
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
