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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	_ "github.com/mattn/go-sqlite3"

	bistrogit "github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type fakeRepoLocationCloner struct {
	mu                sync.Mutex
	calls             []fakeRepoLocationCloneCall
	err               error
	overrideRemoteURL string
	onFailure         func(destination string)
}

type fakeRepoLocationCloneCall struct {
	RepoURL     string
	Destination string
	Bare        bool
}

func (c *fakeRepoLocationCloner) CloneRepo(ctx context.Context, repoURL, destination string, bare bool) error {
	c.mu.Lock()
	c.calls = append(c.calls, fakeRepoLocationCloneCall{RepoURL: repoURL, Destination: destination, Bare: bare})
	err := c.err
	remoteURL := c.overrideRemoteURL
	c.mu.Unlock()
	if err != nil {
		_ = os.MkdirAll(filepath.Join(destination, ".git"), 0o700)
		_ = os.WriteFile(filepath.Join(destination, ".git", "partial"), []byte("created by failed clone"), 0o600)
		if c.onFailure != nil {
			c.onFailure(destination)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if remoteURL == "" {
		remoteURL = repoURL
	}
	return createRepo(destination, "origin", remoteURL)
}

func (c *fakeRepoLocationCloner) Calls() []fakeRepoLocationCloneCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]fakeRepoLocationCloneCall{}, c.calls...)
}

type fakeRepoLocationCacheDeleter struct {
	mu      sync.Mutex
	failed  map[string]error
	deleted []string
}

func (d *fakeRepoLocationCacheDeleter) DeleteCacheDir(path string) error {
	// Callers pass raw row paths or canonicalized stat paths depending on the
	// handler; canonicalize before matching so the injected failure fires
	// regardless of platform tempdir symlinks (macOS /var vs /private/var).
	canonical := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		canonical = resolved
	}
	d.mu.Lock()
	d.deleted = append(d.deleted, path)
	err := d.failed[canonical]
	if err != nil {
		// Fail once: the test models a transient failure (prune fails, the
		// row is preserved, a later clear-all succeeds and removes it).
		delete(d.failed, canonical)
	}
	d.mu.Unlock()
	if err != nil {
		return err
	}
	return os.RemoveAll(path)
}

type fakeRepoLocationSweeper struct {
	removed []string
	err     error
}

func (s *fakeRepoLocationSweeper) SweepOrphans(known map[string]bool) ([]string, error) {
	return append([]string{}, s.removed...), s.err
}

func boolPtr(value bool) *bool {
	return &value
}

func TestRepoLocationValidateDevFolderReturnsCanonicalPathAndEnvelope(t *testing.T) {
	// Arrange
	root := t.TempDir()
	valid := filepath.Join(root, "dev")
	mustMkdirAll(valid)
	missing := filepath.Join(root, "missing")
	handler := NewValidateDevFolderHandler(testRepoLocationLogger())

	tests := []struct {
		name     string
		path     string
		wantOK   bool
		wantCode string
	}{
		{name: "valid", path: valid, wantOK: true},
		{name: "missing", path: missing, wantOK: false, wantCode: "invalid_path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Act
			resp := callRepoLocationHandler(t, handler, "VALIDATE_DEV_FOLDER", map[string]any{"path": tt.path})
			data := responseData(t, resp)

			// Assert
			if got := data["ok"].(bool); got != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v in %#v", tt.wantOK, got, data)
			}
			if tt.wantOK {
				if data["canonical_path"] == "" {
					t.Fatalf("expected canonical_path in %#v", data)
				}
				return
			}
			assertLocationEnvelope(t, data, "", "validate_dev_folder", tt.wantCode)
		})
	}
}

func TestRepoLocationScanDevFolderReturnsInvalidPathEnvelope(t *testing.T) {
	// Arrange
	handler := NewScanDevFolderHandler(store.NewRepoLocationStore(newRepoLocationMigratedDB(t).DB), nil, testRepoLocationLogger())
	missing := filepath.Join(t.TempDir(), "missing")

	// Act
	resp := callRepoLocationHandler(t, handler, "SCAN_DEV_FOLDER", map[string]any{"path": missing, "reason": "test"})

	// Assert
	assertLocationEnvelope(t, responseData(t, resp), "", "scan_dev_folder", "invalid_path")
}

func TestRepoLocationScanDevFolderNoopsWhenBistroRepoDirIsSet(t *testing.T) {
	// Arrange
	root := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", filepath.Join(root, "cache"))
	db := newRepoLocationMigratedDB(t)
	store := store.NewRepoLocationStore(db.DB)
	dev := filepath.Join(root, "dev")
	if err := createRepo(filepath.Join(dev, "acme", "api"), "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatalf("create dev repo: %v", err)
	}
	syncTrigger := make(chan string, 1)
	handler := NewScanDevFolderHandler(store, syncTrigger, testRepoLocationLogger())

	// Act
	resp := callRepoLocationHandler(t, handler, "SCAN_DEV_FOLDER", map[string]any{"path": dev, "reason": "test"})
	data := responseData(t, resp)

	// Assert
	if data["ok"] != true || data["discovered"].(float64) != 0 || data["healthy"].(float64) != 0 || data["degraded"].(float64) != 0 || data["skipped"].(float64) != 0 || data["partial"].(bool) {
		t.Fatalf("expected no-op scan response, got %#v", data)
	}
	location, err := store.Get(context.Background(), "github:acme/api")
	if err != nil {
		t.Fatalf("get repo location: %v", err)
	}
	if location != nil {
		t.Fatalf("expected no local registration with BISTRO_REPO_DIR set, got %+v", location)
	}
	select {
	case <-syncTrigger:
		t.Fatal("expected no sync trigger for env-bypassed scan")
	default:
	}
}

func TestRepoLocationScanDevFolderReconcilesHealthAndTriggersSync(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 permission checks are not reliable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission-denied checks are not meaningful when running as root")
	}
	// Arrange
	db := newRepoLocationMigratedDB(t)
	store := store.NewRepoLocationStore(db.DB)
	dev := filepath.Join(t.TempDir(), "dev")
	mustMkdirAll(dev)

	healthyPath := filepath.Join(dev, "acme", "healthy")
	if err := createRepo(healthyPath, "origin", "https://github.com/acme/healthy.git"); err != nil {
		t.Fatalf("create healthy repo: %v", err)
	}
	missingPath := filepath.Join(dev, "acme", "missing")
	if err := store.UpsertLocal(context.Background(), "github", "acme", "missing", missingPath); err != nil {
		t.Fatalf("upsert missing: %v", err)
	}
	permissionPath := filepath.Join(dev, "acme", "permission")
	if err := createRepo(permissionPath, "origin", "https://github.com/acme/permission.git"); err != nil {
		t.Fatalf("create permission repo: %v", err)
	}
	if err := store.UpsertLocal(context.Background(), "github", "acme", "permission", permissionPath); err != nil {
		t.Fatalf("upsert permission: %v", err)
	}
	mustChmod(t, permissionPath, 0)
	if _, err := os.Open(permissionPath); !os.IsPermission(err) {
		t.Skipf("chmod 000 did not produce permission denial: %v", err)
	}
	mismatchPath := filepath.Join(dev, "acme", "mismatch")
	if err := createRepo(mismatchPath, "origin", "https://github.com/acme/other.git"); err != nil {
		t.Fatalf("create mismatch repo: %v", err)
	}
	if err := store.UpsertLocal(context.Background(), "github", "acme", "mismatch", mismatchPath); err != nil {
		t.Fatalf("upsert mismatch: %v", err)
	}
	syncTrigger := make(chan string, 1)
	handler := NewScanDevFolderHandler(store, syncTrigger, testRepoLocationLogger())

	// Act
	resp := callRepoLocationHandler(t, handler, "SCAN_DEV_FOLDER", map[string]any{"path": dev, "reason": "test"})
	data := responseData(t, resp)

	// Assert
	if data["ok"] != true {
		t.Fatalf("expected ok response, got %#v", data)
	}
	if data["healthy"].(float64) != 2 || data["degraded"].(float64) != 3 {
		t.Fatalf("expected 2 healthy and 3 degraded, got %#v", data)
	}
	assertLocationStatus(t, store, "github:acme/healthy", "ok", "")
	assertLocationStatus(t, store, "github:acme/missing", "missing", "path_missing")
	assertLocationStatus(t, store, "github:acme/permission", "permission_denied", "permission_denied")
	assertLocationStatus(t, store, "github:acme/mismatch", "remote_mismatch", "remote_mismatch")
	select {
	case <-syncTrigger:
	case <-time.After(time.Second):
		t.Fatal("expected scan to trigger sync loop")
	}
}

func TestRepoLocationAddRepoByURL(t *testing.T) {
	tests := []struct {
		name              string
		payload           func(root string) map[string]any
		arrange           func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner)
		wantOK            bool
		wantCode          string
		wantSource        string
		wantPartialClean  string
		wantPreservePath  string
		wantContextCancel bool
		wantStoredSource  string
		wantStoredPath    string
		wantCloneBare     *bool
	}{
		{
			name: "found local via targeted dev folder check",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": nil, "dev_folder_path": filepath.Join(root, "dev")}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				if err := createRepo(filepath.Join(root, "dev", "acme", "api"), "origin", "https://github.com/acme/api.git"); err != nil {
					t.Fatalf("create targeted repo: %v", err)
				}
			},
			wantOK:     true,
			wantSource: "local",
		},
		{
			name: "destination required",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": nil, "dev_folder_path": nil}
			},
			wantOK:   false,
			wantCode: "destination_required",
		},
		{
			name: "invalid url",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "not a url", "destination": nil, "dev_folder_path": nil}
			},
			wantOK:   false,
			wantCode: "invalid_url",
		},
		{
			name: "registered local wins",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "cache", "dev_folder_path": nil}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				localPath := filepath.Join(root, "dev", "acme", "api")
				if err := createRepo(localPath, "origin", "https://github.com/acme/api.git"); err != nil {
					t.Fatalf("create local repo: %v", err)
				}
				if err := store.NewRepoLocationStore(db.DB).UpsertLocal(context.Background(), "github", "acme", "api", localPath); err != nil {
					t.Fatalf("upsert local: %v", err)
				}
			},
			wantOK:     true,
			wantSource: "local",
		},
		{
			name: "degraded registered local returns structured error and preserves row",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "cache", "dev_folder_path": nil}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				localPath := filepath.Join(root, "dev", "acme", "api")
				insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "local", localPath, nil)
				if err := store.NewRepoLocationStore(db.DB).MarkDegraded(context.Background(), "github:acme/api", "missing", "path_missing", "folder not found"); err != nil {
					t.Fatalf("mark degraded local: %v", err)
				}
			},
			wantOK:           false,
			wantCode:         "path_missing",
			wantStoredSource: "local",
			wantStoredPath:   filepath.Join("dev", "acme", "api"),
		},
		{
			name: "cache existing match returns cache",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "cache", "dev_folder_path": nil}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cachePath := filepath.Join(root, "cache", "acme", "api")
				if err := createRepo(cachePath, "origin", "https://github.com/acme/api.git"); err != nil {
					t.Fatalf("create cache repo: %v", err)
				}
			},
			wantOK:           true,
			wantSource:       "cache",
			wantStoredSource: "cache",
		},
		{
			name: "auth failure maps auth_failed",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "cache", "dev_folder_path": nil}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = errors.New("authentication required")
			},
			wantOK:        false,
			wantCode:      "auth_failed",
			wantCloneBare: boolPtr(true),
		},
		{
			name: "unsupported destination is conflict",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "elsewhere", "dev_folder_path": nil}
			},
			wantOK:   false,
			wantCode: "destination_conflict",
		},
		{
			name: "clone failed preserves preexisting empty dir",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "dev_folder", "dev_folder_path": filepath.Join(root, "dev")}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = errors.New("clone boom")
				mustMkdirAll(filepath.Join(root, "dev"))
				mustMkdirAll(filepath.Join(root, "dev", "acme", "api"))
			},
			wantOK:           false,
			wantCode:         "clone_failed",
			wantPreservePath: filepath.Join("dev", "acme", "api"),
		},
		{
			name: "missing dev folder path is invalid",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "dev_folder", "dev_folder_path": filepath.Join(root, "missing-dev")}
			},
			wantOK:   false,
			wantCode: "invalid_path",
		},
		{
			name: "destination conflict",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "dev_folder", "dev_folder_path": filepath.Join(root, "dev")}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				mustMkdirAll(filepath.Join(root, "dev"))
				conflict := filepath.Join(root, "dev", "acme", "api")
				mustMkdirAll(conflict)
				mustWriteFile(filepath.Join(conflict, "README.md"), "not this repo")
			},
			wantOK:   false,
			wantCode: "destination_conflict",
		},
		{
			name: "clone failed cleans partial dir",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "dev_folder", "dev_folder_path": filepath.Join(root, "dev")}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = errors.New("clone boom")
				mustMkdirAll(filepath.Join(root, "dev"))
			},
			wantOK:           false,
			wantCode:         "clone_failed",
			wantPartialClean: filepath.Join("dev", "acme", "api"),
			wantCloneBare:    boolPtr(false),
		},
		{
			name: "clone failed preserves dev folder user file created during clone",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "dev_folder", "dev_folder_path": filepath.Join(root, "dev")}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = errors.New("clone boom")
				mustMkdirAll(filepath.Join(root, "dev"))
				cloner.onFailure = func(destination string) {
					mustWriteFile(filepath.Join(destination, "user-notes.txt"), "created by user during clone")
				}
			},
			wantOK:           false,
			wantCode:         "clone_failed",
			wantPreservePath: filepath.Join("dev", "acme", "api"),
			wantCloneBare:    boolPtr(false),
		},
		{
			name: "post clone remote mismatch cleans partial dir",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "dev_folder", "dev_folder_path": filepath.Join(root, "dev")}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				mustMkdirAll(filepath.Join(root, "dev"))
				cloner.overrideRemoteURL = "https://github.com/acme/other.git"
			},
			wantOK:           false,
			wantCode:         "remote_mismatch",
			wantPartialClean: filepath.Join("dev", "acme", "api"),
		},
		{
			name: "cancelled context",
			payload: func(root string) map[string]any {
				return map[string]any{"url": "https://github.com/acme/api.git", "destination": "dev_folder", "dev_folder_path": filepath.Join(root, "dev")}
			},
			arrange: func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner) {
				cloner.err = context.Canceled
				mustMkdirAll(filepath.Join(root, "dev"))
			},
			wantOK:   false,
			wantCode: "cancelled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			root := t.TempDir()
			db := newRepoLocationMigratedDB(t)
			cloner := &fakeRepoLocationCloner{}
			if tt.arrange != nil {
				tt.arrange(t, db, root, cloner)
			}
			handler := NewAddRepoByURLHandler(AddRepoByURLDeps{
				DB:    db.DB,
				Store: store.NewRepoLocationStore(db.DB),
				Clone: cloner.CloneRepo,
				CacheRoot: func() (string, error) {
					cacheRoot := filepath.Join(root, "cache")
					mustMkdirAll(cacheRoot)
					return cacheRoot, nil
				},
				Log: testRepoLocationLogger(),
			})
			ctx := context.Background()
			if tt.wantContextCancel {
				cancelCtx, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelCtx
			}

			// Act
			resp := callRepoLocationHandlerWithContext(t, ctx, handler, "ADD_REPO_BY_URL", tt.payload(root))
			data := responseData(t, resp)

			// Assert
			if data["ok"].(bool) != tt.wantOK {
				t.Fatalf("expected ok=%v, got %#v", tt.wantOK, data)
			}
			if tt.wantOK {
				if data["repo_id"] != "github:acme/api" || data["source"] != tt.wantSource {
					t.Fatalf("unexpected success payload %#v", data)
				}
				if tt.wantStoredSource != "" {
					storedPath := filepath.Join(root, "cache", "acme", "api")
					if tt.wantStoredPath != "" {
						storedPath = filepath.Join(root, tt.wantStoredPath)
					}
					assertSourcePath(t, db.DB, "github:acme/api", tt.wantStoredSource, storedPath)
				}
				return
			}
			wantRepoID := "github:acme/api"
			if tt.wantCode == "invalid_url" {
				wantRepoID = ""
			}
			assertLocationEnvelope(t, data, wantRepoID, "add_repo_by_url", tt.wantCode)
			if tt.wantStoredSource != "" {
				storedPath := filepath.Join(root, "cache", "acme", "api")
				if tt.wantStoredPath != "" {
					storedPath = filepath.Join(root, tt.wantStoredPath)
				}
				assertSourcePath(t, db.DB, "github:acme/api", tt.wantStoredSource, storedPath)
			}
			if tt.wantPartialClean != "" {
				if _, err := os.Stat(filepath.Join(root, tt.wantPartialClean)); !os.IsNotExist(err) {
					t.Fatalf("expected partial dir cleanup, stat err=%v", err)
				}
			}
			if tt.wantPreservePath != "" {
				if _, err := os.Stat(filepath.Join(root, tt.wantPreservePath)); err != nil {
					t.Fatalf("expected preexisting dir preserved, stat err=%v", err)
				}
			}
		})
	}
}

func TestRepoLocationAddRepoByURLUsesCacheRootWhenBistroRepoDirIsSet(t *testing.T) {
	// Arrange
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)
	dev := filepath.Join(root, "dev")
	if err := createRepo(filepath.Join(dev, "acme", "api"), "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatalf("create targeted dev repo: %v", err)
	}
	db := newRepoLocationMigratedDB(t)
	cloner := &fakeRepoLocationCloner{}
	handler := NewAddRepoByURLHandler(AddRepoByURLDeps{
		DB:        db.DB,
		Store:     store.NewRepoLocationStore(db.DB),
		Clone:     cloner.CloneRepo,
		CacheRoot: bistrogit.CacheRoot,
		Log:       testRepoLocationLogger(),
	})

	// Act
	resp := callRepoLocationHandler(t, handler, "ADD_REPO_BY_URL", map[string]any{"url": "https://github.com/acme/api.git", "destination": "dev_folder", "dev_folder_path": dev})
	data := responseData(t, resp)

	// Assert
	if data["ok"] != true || data["source"] != "cache" {
		t.Fatalf("expected cache success with env override, got %#v", data)
	}
	assertSourcePath(t, db.DB, "github:acme/api", "cache", filepath.Join(cacheRoot, "acme", "api"))
	calls := cloner.Calls()
	if len(calls) != 1 || calls[0].Destination != filepath.Join(cacheRoot, "acme", "api") || !calls[0].Bare {
		t.Fatalf("expected bare clone into env cache root, got %#v", calls)
	}
}

func TestRepoLocationCacheMutationHandlersHandleMissingDeleteDependency(t *testing.T) {
	// Arrange
	root := t.TempDir()
	db := newRepoLocationMigratedDB(t)
	stale := time.Now().Add(-31 * 24 * time.Hour)
	cachePath := filepath.Join(root, "cache", "acme", "api")
	mustMkdirAll(cachePath)
	mustWriteFile(filepath.Join(cachePath, "payload"), "bytes")
	insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "cache", cachePath, &stale)
	deps := CacheMaintenanceDeps{Store: store.NewRepoLocationStore(db.DB), Locks: bistrogit.NewRepoLocks(), Log: testRepoLocationLogger()}

	// Act
	pruneResp := callRepoLocationHandler(t, NewPruneCacheHandler(deps), "PRUNE_CACHE", map[string]any{})
	clearResp := callRepoLocationHandler(t, NewClearCacheHandler(deps), "CLEAR_CACHE", map[string]any{})
	deleteResp := callRepoLocationHandler(t, NewDeleteCacheRepoHandler(deps), "DELETE_CACHE_REPO", map[string]any{"repo_id": "github:acme/api"})

	// Assert
	assertFailedRepo(t, responseData(t, pruneResp)["failed"].([]any), "github:acme/api", "delete_failed")
	assertFailedRepo(t, responseData(t, clearResp)["failed"].([]any), "github:acme/api", "delete_failed")
	assertLocationEnvelope(t, responseData(t, deleteResp), "github:acme/api", "delete_cache_repo", "delete_failed")
	assertSourcePath(t, db.DB, "github:acme/api", "cache", cachePath)
}

func TestRepoLocationCacheMutationHandlersReturnEnvelopeForOperationFailures(t *testing.T) {
	tests := []struct {
		name      string
		msgType   string
		operation string
		handler   func(CacheMaintenanceDeps) ws.MessageHandler
		deps      func(*store.RepoLocationStore) CacheMaintenanceDeps
	}{
		{
			name:      "prune cache store failure",
			msgType:   "PRUNE_CACHE",
			operation: "prune_cache",
			handler:   NewPruneCacheHandler,
			deps: func(locations *store.RepoLocationStore) CacheMaintenanceDeps {
				return CacheMaintenanceDeps{Store: locations, Locks: bistrogit.NewRepoLocks(), DeleteCacheDir: func(string) error { return nil }, Log: testRepoLocationLogger()}
			},
		},
		{
			name:      "clear cache store failure",
			msgType:   "CLEAR_CACHE",
			operation: "clear_cache",
			handler:   NewClearCacheHandler,
			deps: func(locations *store.RepoLocationStore) CacheMaintenanceDeps {
				return CacheMaintenanceDeps{Store: locations, Locks: bistrogit.NewRepoLocks(), DeleteCacheDir: func(string) error { return nil }, Log: testRepoLocationLogger()}
			},
		},
		{
			name:      "clear cache orphan sweep failure",
			msgType:   "CLEAR_CACHE",
			operation: "clear_cache",
			handler:   NewClearCacheHandler,
			deps: func(locations *store.RepoLocationStore) CacheMaintenanceDeps {
				return CacheMaintenanceDeps{
					Store:          locations,
					Locks:          bistrogit.NewRepoLocks(),
					DeleteCacheDir: func(string) error { return nil },
					SweepOrphans:   (&fakeRepoLocationSweeper{err: errors.New("sweep failed")}).SweepOrphans,
					Log:            testRepoLocationLogger(),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			db := newRepoLocationMigratedDB(t)
			locations := store.NewRepoLocationStore(db.DB)
			if strings.Contains(tt.name, "store failure") {
				if err := db.Close(); err != nil {
					t.Fatalf("close db: %v", err)
				}
			}
			handler := tt.handler(tt.deps(locations))

			// Act
			resp := callRepoLocationHandler(t, handler, tt.msgType, map[string]any{})

			// Assert
			assertLocationEnvelope(t, responseData(t, resp), "", tt.operation, "delete_failed")
		})
	}
}

func TestRepoLocationRemoveRepoPreflightAndRemoveRepo(t *testing.T) {
	// Arrange
	db := newRepoLocationMigratedDB(t)
	insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "cache", filepath.Join(t.TempDir(), "acme", "api"), nil)
	insertRepoWithLocation(t, db, "github:acme/other", "github", "acme", "other", "local", filepath.Join(t.TempDir(), "acme", "other"), nil)
	insertPRGraph(t, db, "pr1", "github:acme/api", "github", "acme", "api")
	insertPRGraph(t, db, "pr2", "github:acme/api", "github", "acme", "api")
	insertPRGraph(t, db, "pr3", "github:acme/other", "github", "acme", "other")
	deleteCalls := 0
	preflight := NewRemoveRepoPreflightHandler(db.DB, testRepoLocationLogger())
	remove := NewRemoveRepoHandler(RemoveRepoDeps{
		DB:    db.DB,
		Store: store.NewRepoLocationStore(db.DB),
		DeleteCacheDir: func(path string) error {
			deleteCalls++
			return nil
		},
		Log: testRepoLocationLogger(),
	})

	// Act
	preflightResp := callRepoLocationHandler(t, preflight, "REMOVE_REPO_PREFLIGHT", map[string]any{"repo_id": "github:acme/api"})
	removeResp := callRepoLocationHandler(t, remove, "REMOVE_REPO", map[string]any{"repo_id": "github:acme/api"})

	// Assert
	preflightData := responseData(t, preflightResp)
	assertCounts(t, preflightData["counts"].(map[string]any), 2, 2, 2, 2, 2)
	removeData := responseData(t, removeResp)
	if removeData["ok"] != true {
		t.Fatalf("expected remove ok, got %#v", removeData)
	}
	assertCounts(t, removeData["deleted"].(map[string]any), 2, 2, 2, 2, 2)
	assertRowCount(t, db.DB, `SELECT COUNT(*) FROM prs WHERE repo_id = ?`, []any{"github:acme/api"}, 0)
	assertRowCount(t, db.DB, `SELECT COUNT(*) FROM prs WHERE repo_id = ?`, []any{"github:acme/other"}, 1)
	if deleteCalls != 1 {
		t.Fatalf("expected one cache delete call, got %d", deleteCalls)
	}
}

func TestRepoLocationRemoveRepoRollsBackOnDeleteFailure(t *testing.T) {
	// Arrange
	db := newRepoLocationMigratedDB(t)
	insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "local", filepath.Join(t.TempDir(), "api"), nil)
	insertPRGraph(t, db, "pr1", "github:acme/api", "github", "acme", "api")
	handler := NewRemoveRepoHandler(RemoveRepoDeps{DB: db.DB, Store: store.NewRepoLocationStore(db.DB), Log: testRepoLocationLogger()})
	_, err := db.DB.Exec(`DROP TABLE action_queue`)
	if err != nil {
		t.Fatalf("drop action_queue: %v", err)
	}

	// Act
	_, callErr := handler(context.Background(), ws.InboundMessage{ID: "1", Type: "REMOVE_REPO", Data: mustJSON(map[string]any{"repo_id": "github:acme/api"})})

	// Assert
	if callErr == nil {
		t.Fatal("expected handler error from injected tx failure")
	}
	assertRowCount(t, db.DB, `SELECT COUNT(*) FROM prs WHERE repo_id = ?`, []any{"github:acme/api"}, 1)
	assertRowCount(t, db.DB, `SELECT COUNT(*) FROM repos WHERE id = ?`, []any{"github:acme/api"}, 1)
}

func TestRepoLocationCacheMaintenance(t *testing.T) {
	// Arrange
	root := t.TempDir()
	t.Setenv("BISTRO_REPO_DIR", filepath.Join(root, "cache"))
	db := newRepoLocationMigratedDB(t)
	localPath := filepath.Join(root, "local", "acme", "api")
	freshPath := filepath.Join(root, "cache", "acme", "fresh")
	stalePath := filepath.Join(root, "cache", "acme", "stale")
	nullPath := filepath.Join(root, "cache", "acme", "null-used")
	failedPath := filepath.Join(root, "cache", "acme", "failed")
	deletePath := filepath.Join(root, "cache", "acme", "delete-one")
	unsafeTarget := filepath.Join(t.TempDir(), "unsafe-target")
	unsafePath := filepath.Join(root, "cache", "acme", "unsafe")
	for _, path := range []string{localPath, freshPath, stalePath, nullPath, failedPath, deletePath, unsafeTarget} {
		mustMkdirAll(path)
		mustWriteFile(filepath.Join(path, "payload"), "bytes")
	}
	if err := os.Symlink(unsafeTarget, unsafePath); err != nil {
		t.Fatalf("create unsafe cache symlink: %v", err)
	}
	fresh := time.Now().Add(-29 * 24 * time.Hour)
	stale := time.Now().Add(-31 * 24 * time.Hour)
	insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "local", localPath, &stale)
	insertRepoWithLocation(t, db, "github:acme/fresh", "github", "acme", "fresh", "cache", freshPath, &fresh)
	insertRepoWithLocation(t, db, "github:acme/stale", "github", "acme", "stale", "cache", stalePath, &stale)
	insertRepoWithLocation(t, db, "github:acme/null-used", "github", "acme", "null-used", "cache", nullPath, nil)
	insertRepoWithLocation(t, db, "github:acme/failed", "github", "acme", "failed", "cache", failedPath, &stale)
	insertRepoWithLocation(t, db, "github:acme/delete-one", "github", "acme", "delete-one", "cache", deletePath, &fresh)
	insertRepoWithLocation(t, db, "github:acme/unsafe", "github", "acme", "unsafe", "cache", unsafePath, &fresh)
	deleteErr := errors.New("delete failed")
	originalFailedPath := failedPath
	canonicalFailedPath, err := filepath.EvalSymlinks(failedPath)
	if err != nil {
		t.Fatalf("canonical failed path: %v", err)
	}
	deleter := &fakeRepoLocationCacheDeleter{failed: map[string]error{canonicalFailedPath: deleteErr}}
	sweeper := &fakeRepoLocationSweeper{removed: []string{filepath.Join(root, "cache", "orphan")}}
	locks := bistrogit.NewRepoLocks()
	deps := CacheMaintenanceDeps{
		Store:          store.NewRepoLocationStore(db.DB),
		Locks:          locks,
		DeleteCacheDir: deleter.DeleteCacheDir,
		SweepOrphans:   sweeper.SweepOrphans,
		Log:            testRepoLocationLogger(),
	}

	// Act: stats
	statsResp := callRepoLocationHandler(t, NewGetCacheStatsHandler(deps), "GET_CACHE_STATS", map[string]any{})
	statsData := responseData(t, statsResp)

	// Assert: stats only include cache repos
	if statsData["ok"] != true || statsData["total_bytes"].(float64) <= 0 {
		t.Fatalf("unexpected stats payload %#v", statsData)
	}
	assertStatsRepoPresence(t, statsData["repos"].([]any), map[string]bool{
		"github:acme/fresh":      true,
		"github:acme/stale":      true,
		"github:acme/null-used":  true,
		"github:acme/failed":     true,
		"github:acme/delete-one": true,
		"github:acme/unsafe":     false,
		"github:acme/api":        false,
	})
	assertStatsRepoShape(t, statsData["repos"].([]any))

	// Act: prune
	pruneResp := callRepoLocationHandler(t, NewPruneCacheHandler(deps), "PRUNE_CACHE", map[string]any{})
	pruneData := responseData(t, pruneResp)

	// Assert: prune respects staleness and local rows
	assertStringSet(t, pruneData["removed"].([]any), []string{"github:acme/stale", "github:acme/null-used"})
	assertFailedRepo(t, pruneData["failed"].([]any), "github:acme/failed", "delete_failed")
	assertSourcePath(t, db.DB, "github:acme/api", "local", localPath)
	assertSourcePath(t, db.DB, "github:acme/fresh", "cache", freshPath)
	assertSourcePath(t, db.DB, "github:acme/stale", "", "")
	assertSourcePath(t, db.DB, "github:acme/null-used", "", "")
	assertSourcePath(t, db.DB, "github:acme/failed", "cache", originalFailedPath)

	// Act: delete one cache repo
	deleteResp := callRepoLocationHandler(t, NewDeleteCacheRepoHandler(deps), "DELETE_CACHE_REPO", map[string]any{"repo_id": "github:acme/delete-one"})
	deleteData := responseData(t, deleteResp)

	// Assert: delete one clears cache fields
	if deleteData["ok"] != true {
		t.Fatalf("expected delete cache repo ok, got %#v", deleteData)
	}
	assertSourcePath(t, db.DB, "github:acme/delete-one", "", "")

	// Act: local delete refused
	localDeleteResp := callRepoLocationHandler(t, NewDeleteCacheRepoHandler(deps), "DELETE_CACHE_REPO", map[string]any{"repo_id": "github:acme/api"})
	localDeleteData := responseData(t, localDeleteResp)

	// Assert
	assertLocationEnvelope(t, localDeleteData, "github:acme/api", "delete_cache_repo", "delete_failed")

	// Act + Assert: clear cache holds maintenance lock and sweeps orphans
	blockStarted := make(chan struct{})
	blockRelease := make(chan struct{})
	go func() {
		_ = locks.WithRepo("github:acme/concurrent", func() error {
			close(blockStarted)
			<-blockRelease
			return nil
		})
	}()
	<-blockStarted
	clearDone := make(chan map[string]any)
	go func() {
		resp := callRepoLocationHandler(t, NewClearCacheHandler(deps), "CLEAR_CACHE", map[string]any{})
		clearDone <- responseData(t, resp)
	}()
	select {
	case <-clearDone:
		t.Fatal("clear cache should block behind active repo operation")
	case <-time.After(100 * time.Millisecond):
	}
	close(blockRelease)
	clearData := <-clearDone
	if clearData["ok"] != true || clearData["orphans_removed"].(float64) != 1 {
		t.Fatalf("unexpected clear cache payload %#v", clearData)
	}
	assertStringSet(t, clearData["removed"].([]any), []string{"github:acme/fresh", "github:acme/failed", "github:acme/unsafe"})
}

func TestRepoLocationDeleteCacheRepoRereadsAfterWaitingForLock(t *testing.T) {
	// Arrange
	root := t.TempDir()
	cacheRoot := filepath.Join(root, "cache")
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)
	db := newRepoLocationMigratedDB(t)
	cachePath := filepath.Join(cacheRoot, "acme", "api")
	localPath := filepath.Join(root, "dev", "acme", "api")
	mustMkdirAll(cachePath)
	mustWriteFile(filepath.Join(cachePath, "cached"), "cache payload")
	mustMkdirAll(localPath)
	insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "cache", cachePath, nil)
	locks := bistrogit.NewRepoLocks()
	lockHeld := make(chan struct{})
	lockRelease := make(chan struct{})
	go func() {
		_ = locks.WithRepo("github:acme/api", func() error {
			close(lockHeld)
			<-lockRelease
			return nil
		})
	}()
	<-lockHeld
	deleteCalls := 0
	handlerDone := make(chan map[string]any)
	handler := NewDeleteCacheRepoHandler(CacheMaintenanceDeps{
		Store: store.NewRepoLocationStore(db.DB),
		Locks: locks,
		DeleteCacheDir: func(path string) error {
			deleteCalls++
			return os.RemoveAll(path)
		},
		Log: testRepoLocationLogger(),
	})

	// Act
	go func() {
		resp := callRepoLocationHandler(t, handler, "DELETE_CACHE_REPO", map[string]any{"repo_id": "github:acme/api"})
		handlerDone <- responseData(t, resp)
	}()
	select {
	case data := <-handlerDone:
		t.Fatalf("delete should wait for repo lock before completing, got %#v", data)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := db.Exec(`UPDATE repos SET source = 'local', local_path = ? WHERE id = ?`, localPath, "github:acme/api"); err != nil {
		t.Fatalf("change repo row while delete waits: %v", err)
	}
	close(lockRelease)
	data := <-handlerDone

	// Assert
	assertLocationEnvelope(t, data, "github:acme/api", "delete_cache_repo", "delete_failed")
	if deleteCalls != 0 {
		t.Fatalf("expected no stale cache deletion after row changed, got %d calls", deleteCalls)
	}
	assertSourcePath(t, db.DB, "github:acme/api", "local", localPath)
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("expected cache path preserved after stale-row abort, stat err=%v", err)
	}
}

func TestRepoLocationRecoverRepoPathAndToCache(t *testing.T) {
	// Arrange
	root := t.TempDir()
	db := newRepoLocationMigratedDB(t)
	store := store.NewRepoLocationStore(db.DB)
	missingPath := filepath.Join(root, "missing")
	insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "local", missingPath, nil)
	if err := store.MarkDegraded(context.Background(), "github:acme/api", "missing", "path_missing", "folder not found"); err != nil {
		t.Fatalf("mark degraded: %v", err)
	}
	goodPath := filepath.Join(root, "dev", "acme", "api")
	if err := createRepo(goodPath, "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatalf("create good repo: %v", err)
	}
	badPath := filepath.Join(root, "dev", "acme", "bad")
	if err := createRepo(badPath, "origin", "https://github.com/acme/bad.git"); err != nil {
		t.Fatalf("create bad repo: %v", err)
	}
	cloner := &fakeRepoLocationCloner{}
	recoverPath := NewRecoverRepoPathHandler(store, testRepoLocationLogger())
	recoverCache := NewRecoverRepoToCacheHandler(RecoverRepoToCacheDeps{
		DB:    db.DB,
		Store: store,
		Clone: cloner.CloneRepo,
		CacheRoot: func() (string, error) {
			cacheRoot := filepath.Join(root, "cache")
			mustMkdirAll(cacheRoot)
			return cacheRoot, nil
		},
		Log: testRepoLocationLogger(),
	})

	// Act + Assert: post-clone validation mismatch maps to clone_failed and preserves degraded state.
	cloner.overrideRemoteURL = "https://github.com/acme/other.git"
	cacheMismatchResp := callRepoLocationHandler(t, recoverCache, "RECOVER_REPO_TO_CACHE", map[string]any{"repo_id": "github:acme/api"})
	assertLocationEnvelope(t, responseData(t, cacheMismatchResp), "github:acme/api", "recover_repo_to_cache", "clone_failed")
	assertLocationStatus(t, store, "github:acme/api", "missing", "path_missing")
	if _, err := os.Stat(filepath.Join(root, "cache", "acme", "api")); !os.IsNotExist(err) {
		t.Fatalf("expected mismatched clone cleanup, stat err=%v", err)
	}
	cloner.overrideRemoteURL = ""

	// Act + Assert: mismatch envelope
	mismatchResp := callRepoLocationHandler(t, recoverPath, "RECOVER_REPO_PATH", map[string]any{"repo_id": "github:acme/api", "path": badPath})
	assertLocationEnvelope(t, responseData(t, mismatchResp), "github:acme/api", "recover_repo_path", "remote_mismatch")

	// Act + Assert: good path clears degraded state
	goodResp := callRepoLocationHandler(t, recoverPath, "RECOVER_REPO_PATH", map[string]any{"repo_id": "github:acme/api", "path": goodPath})
	if data := responseData(t, goodResp); data["ok"] != true {
		t.Fatalf("expected recover path ok, got %#v", data)
	}
	assertLocationStatus(t, store, "github:acme/api", "ok", "")

	// Act + Assert: missing path envelope
	missingResp := callRepoLocationHandler(t, recoverPath, "RECOVER_REPO_PATH", map[string]any{"repo_id": "github:acme/api", "path": filepath.Join(root, "does-not-exist")})
	assertLocationEnvelope(t, responseData(t, missingResp), "github:acme/api", "recover_repo_path", "path_missing")

	// Act + Assert: permission denied path envelope
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		permissionPath := filepath.Join(root, "dev", "acme", "permission-denied")
		if err := createRepo(permissionPath, "origin", "https://github.com/acme/api.git"); err != nil {
			t.Fatalf("create permission repo: %v", err)
		}
		mustChmod(t, permissionPath, 0)
		if _, err := os.Open(permissionPath); !os.IsPermission(err) {
			t.Skipf("chmod 000 did not produce permission denial: %v", err)
		}
		permissionResp := callRepoLocationHandler(t, recoverPath, "RECOVER_REPO_PATH", map[string]any{"repo_id": "github:acme/api", "path": permissionPath})
		assertLocationEnvelope(t, responseData(t, permissionResp), "github:acme/api", "recover_repo_path", "permission_denied")
	}

	// Act + Assert: existing file path envelope
	existingFilePath := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(existingFilePath, []byte("not a git repo directory"), 0o600); err != nil {
		t.Fatalf("write existing file path fixture: %v", err)
	}
	fileResp := callRepoLocationHandler(t, recoverPath, "RECOVER_REPO_PATH", map[string]any{"repo_id": "github:acme/api", "path": existingFilePath})
	assertLocationEnvelope(t, responseData(t, fileResp), "github:acme/api", "recover_repo_path", "path_missing")

	// Arrange degraded again for cache recovery
	if err := store.MarkDegraded(context.Background(), "github:acme/api", "missing", "path_missing", "folder not found"); err != nil {
		t.Fatalf("mark degraded: %v", err)
	}

	// Act + Assert: clone failure preserves degraded state and preexisting empty dir.
	preexistingCachePath := filepath.Join(root, "cache", "acme", "api")
	mustMkdirAll(preexistingCachePath)
	cloner.err = errors.New("clone failed")
	failureResp := callRepoLocationHandler(t, recoverCache, "RECOVER_REPO_TO_CACHE", map[string]any{"repo_id": "github:acme/api"})
	assertLocationEnvelope(t, responseData(t, failureResp), "github:acme/api", "recover_repo_to_cache", "clone_failed")
	assertLocationStatus(t, store, "github:acme/api", "missing", "path_missing")
	if _, err := os.Stat(preexistingCachePath); err != nil {
		t.Fatalf("expected preexisting cache dir preserved, stat err=%v", err)
	}
	if err := os.RemoveAll(preexistingCachePath); err != nil {
		t.Fatalf("remove preserved cache dir: %v", err)
	}
	cloner.err = nil

	// Act
	cacheResp := callRepoLocationHandler(t, recoverCache, "RECOVER_REPO_TO_CACHE", map[string]any{"repo_id": "github:acme/api"})
	cacheData := responseData(t, cacheResp)

	// Assert
	if cacheData["ok"] != true {
		t.Fatalf("expected recover cache ok, got %#v", cacheData)
	}
	location, err := store.Get(context.Background(), "github:acme/api")
	if err != nil {
		t.Fatalf("get recovered cache: %v", err)
	}
	if location.Source != "cache" || location.LocationStatus != "ok" {
		t.Fatalf("expected cache ok location, got %+v", location)
	}
}

func callRepoLocationHandler(t *testing.T, handler ws.MessageHandler, messageType string, payload map[string]any) map[string]any {
	t.Helper()
	return callRepoLocationHandlerWithContext(t, context.Background(), handler, messageType, payload)
}

func callRepoLocationHandlerWithContext(t *testing.T, ctx context.Context, handler ws.MessageHandler, messageType string, payload map[string]any) map[string]any {
	t.Helper()
	resp, err := handler(ctx, ws.InboundMessage{ID: "msg-1", Type: messageType, Data: mustJSON(payload)})
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	response, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", resp)
	}
	return response
}

func responseData(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	bytes, err := json.Marshal(resp["data"])
	if err != nil {
		t.Fatalf("marshal response data: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(bytes, &data); err != nil {
		t.Fatalf("unmarshal response data: %v", err)
	}
	return data
}

func assertLocationEnvelope(t *testing.T, data map[string]any, repoID string, operation string, code string) {
	t.Helper()
	if data["ok"] != false {
		t.Fatalf("expected ok=false envelope, got %#v", data)
	}
	for _, field := range []string{"repo_id", "operation", "code", "message"} {
		if _, ok := data[field]; !ok {
			t.Fatalf("expected envelope field %q in %#v", field, data)
		}
	}
	if data["repo_id"] != repoID || data["operation"] != operation || data["code"] != code || data["message"] == "" {
		t.Fatalf("unexpected envelope: %#v", data)
	}
}

func newRepoLocationMigratedDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	runner := store.NewMigrationRunner(nil, db.DB)
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func insertRepoWithLocation(t *testing.T, db *store.DB, repoID, provider, owner, repo, source, localPath string, lastUsedAt *time.Time) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO repos (id, provider, owner, repo, watched, local_path, source, last_used_at, location_status) VALUES (?, ?, ?, ?, 1, ?, ?, ?, 'ok')`, repoID, provider, owner, repo, nullableString(localPath), nullableString(source), lastUsedAt)
	if err != nil {
		t.Fatalf("insert repo %s: %v", repoID, err)
	}
}

func insertPRGraph(t *testing.T, db *store.DB, prID, repoID, provider, owner, repo string) {
	t.Helper()
	prNumber := 1
	_, _ = fmt.Sscanf(strings.TrimPrefix(prID, "pr"), "%d", &prNumber)
	_, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state, created_at, updated_at, base_branch, head_branch, base_sha, head_sha, repo_id) VALUES (?, ?, ?, ?, ?, 'title', 'author', 'open', ?, ?, 'main', 'feature', 'base', 'head', ?)`, prID, provider, owner, repo, prNumber, time.Now(), time.Now(), repoID)
	if err != nil {
		t.Fatalf("insert pr %s: %v", prID, err)
	}
	statements := []string{
		`INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, thread_id) VALUES (?, ?, 'author', 'body', ?, ?, ?)`,
		`INSERT INTO review_threads (id, pr_id, file_path) VALUES (?, ?, 'file.go')`,
		`INSERT INTO patchsets (pr_id, number, head_sha, base_sha, pushed_at) VALUES (?, 1, ?, ?, ?)`,
		`INSERT INTO last_reviewed_sha (pr_id, account_provider, account_login, login, sha) VALUES (?, 'github', 'me', 'me', ?)`,
		`INSERT INTO files_viewed (pr_id, file_path, account_provider, account_login, login, content_hash) VALUES (?, 'file.go', 'github', 'me', 'me', 'hash')`,
		`INSERT INTO action_queue (id, action_type, pr_id, payload) VALUES (?, 'approve', ?, '{}')`,
	}
	threadID := prID + "-thread"
	values := [][]any{
		{prID + "-comment", prID, time.Now(), time.Now(), threadID},
		{threadID, prID},
		{prID, prID + "-head", prID + "-base", time.Now()},
		{prID, prID + "-sha"},
		{prID},
		{prID + "-action", prID},
	}
	// review_threads must exist before comments.thread_id can reference it.
	for _, idx := range []int{1, 0, 2, 3, 4, 5} {
		if _, err := db.Exec(statements[idx], values[idx]...); err != nil {
			t.Fatalf("insert graph statement %d for %s: %v", idx, prID, err)
		}
	}
}

func assertCounts(t *testing.T, counts map[string]any, prs, comments, threads, patchsets, actions int) {
	t.Helper()
	want := map[string]int{"prs": prs, "comments": comments, "threads": threads, "patchsets": patchsets, "actions": actions}
	for key, value := range want {
		if int(counts[key].(float64)) != value {
			t.Fatalf("expected %s=%d, got %#v", key, value, counts)
		}
	}
}

func assertRowCount(t *testing.T, db *sql.DB, query string, args []any, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("expected count %d, got %d for %q", want, got, query)
	}
}

func assertLocationStatus(t *testing.T, store *store.RepoLocationStore, repoID, wantStatus, wantCode string) {
	t.Helper()
	location, err := store.Get(context.Background(), repoID)
	if err != nil {
		t.Fatalf("get location %s: %v", repoID, err)
	}
	if location == nil {
		t.Fatalf("missing location %s", repoID)
	}
	if location.LocationStatus != wantStatus || location.LocationErrorCode != wantCode {
		t.Fatalf("expected %s/%s, got %+v", wantStatus, wantCode, location)
	}
}

func assertStatsRepoPresence(t *testing.T, repos []any, expected map[string]bool) {
	t.Helper()
	actual := make(map[string]bool, len(repos))
	for _, item := range repos {
		repo := item.(map[string]any)
		actual[repo["repo_id"].(string)] = true
	}
	for repoID, wantPresent := range expected {
		if actual[repoID] != wantPresent {
			t.Fatalf("expected presence %v for %s, got repos %#v", wantPresent, repoID, actual)
		}
	}
}

func assertStatsRepoShape(t *testing.T, repos []any) {
	t.Helper()
	for _, item := range repos {
		repo := item.(map[string]any)
		if _, ok := repo["repo_id"].(string); !ok {
			t.Fatalf("expected repo_id string in stats repo %#v", repo)
		}
		if _, ok := repo["size_bytes"].(float64); !ok {
			t.Fatalf("expected size_bytes number in stats repo %#v", repo)
		}
		if _, ok := repo["last_used_at"]; !ok {
			t.Fatalf("expected last_used_at in stats repo %#v", repo)
		}
		if _, ok := repo["stale"].(bool); !ok {
			t.Fatalf("expected stale bool in stats repo %#v", repo)
		}
		if _, ok := repo["skipped"]; ok {
			t.Fatalf("stats repo must not include skipped field: %#v", repo)
		}
		if _, ok := repo["error"]; ok {
			t.Fatalf("stats repo must not include error field: %#v", repo)
		}
		if len(repo) != 4 {
			t.Fatalf("expected exactly declared stats fields, got %#v", repo)
		}
	}
}

func assertStringSet(t *testing.T, got []any, want []string) {
	t.Helper()
	actual := make(map[string]bool, len(got))
	for _, value := range got {
		actual[value.(string)] = true
	}
	if len(actual) != len(want) {
		t.Fatalf("expected set %v, got %v", want, actual)
	}
	for _, value := range want {
		if !actual[value] {
			t.Fatalf("expected %q in %v", value, actual)
		}
	}
}

func assertFailedRepo(t *testing.T, failed []any, repoID, code string) {
	t.Helper()
	for _, item := range failed {
		failure := item.(map[string]any)
		if failure["repo_id"] == repoID && failure["code"] == code && failure["message"] != "" {
			return
		}
	}
	t.Fatalf("expected failed repo %s/%s in %#v", repoID, code, failed)
}

func assertSourcePath(t *testing.T, db *sql.DB, repoID, wantSource, wantPath string) {
	t.Helper()
	var source, path sql.NullString
	if err := db.QueryRow(`SELECT source, local_path FROM repos WHERE id = ?`, repoID).Scan(&source, &path); err != nil {
		t.Fatalf("select source path %s: %v", repoID, err)
	}
	if source.String != wantSource || source.Valid != (wantSource != "") || path.String != wantPath || path.Valid != (wantPath != "") {
		t.Fatalf("expected source/path %q/%q, got valid=%v %q valid=%v %q", wantSource, wantPath, source.Valid, source.String, path.Valid, path.String)
	}
}

func createRepo(path string, remoteName string, remoteURL string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	repo, err := gogit.PlainInit(path, false)
	if err != nil {
		return err
	}
	_, err = repo.CreateRemote(&config.RemoteConfig{Name: remoteName, URLs: []string{remoteURL}})
	return err
}

func mustMkdirAll(path string) {
	if err := os.MkdirAll(path, 0o700); err != nil {
		panic(fmt.Sprintf("mkdir %s: %v", path, err))
	}
}

func mustWriteFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		panic(fmt.Sprintf("write %s: %v", path, err))
	}
}

func mustChmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
}

func mustJSON(payload map[string]any) json.RawMessage {
	bytes, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return bytes
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func testRepoLocationLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
