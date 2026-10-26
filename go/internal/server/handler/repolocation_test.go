package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
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
	handler := newTestScanDevFolderHandler(t, store.NewRepoLocationStore(storetest.Open(t).DB), nil)
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
	db := storetest.Open(t)
	store := store.NewRepoLocationStore(db.DB)
	dev := filepath.Join(root, "dev")
	if err := createRepo(filepath.Join(dev, "acme", "api"), "origin", "https://github.com/acme/api.git"); err != nil {
		t.Fatalf("create dev repo: %v", err)
	}
	syncTrigger := make(chan string, 1)
	handler := newTestScanDevFolderHandler(t, store, syncTrigger)

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
	db := storetest.Open(t)
	store := store.NewRepoLocationStore(db.DB)
	dev := filepath.Join(t.TempDir(), "dev")
	mustMkdirAll(dev)

	healthyPath := filepath.Join(dev, "acme", "healthy")
	if err := createRepo(healthyPath, "origin", "https://github.com/acme/healthy.git"); err != nil {
		t.Fatalf("create healthy repo: %v", err)
	}
	missingPath := filepath.Join(dev, "acme", "missing")
	if err := store.UpsertLocal(context.Background(), "github", "acme", "missing", missingPath, false); err != nil {
		t.Fatalf("upsert missing: %v", err)
	}
	permissionPath := filepath.Join(dev, "acme", "permission")
	if err := createRepo(permissionPath, "origin", "https://github.com/acme/permission.git"); err != nil {
		t.Fatalf("create permission repo: %v", err)
	}
	if err := store.UpsertLocal(context.Background(), "github", "acme", "permission", permissionPath, false); err != nil {
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
	if err := store.UpsertLocal(context.Background(), "github", "acme", "mismatch", mismatchPath, false); err != nil {
		t.Fatalf("upsert mismatch: %v", err)
	}
	syncTrigger := make(chan string, 1)
	handler := newTestScanDevFolderHandler(t, store, syncTrigger)

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
		name             string
		payload          func(root string) map[string]any
		arrange          func(t *testing.T, db *store.DB, root string, cloner *fakeRepoLocationCloner)
		wantOK           bool
		wantCode         string
		wantSource       string
		wantPartialClean string
		wantPreservePath string
		wantStoredSource string
		wantStoredPath   string
		wantCloneBare    *bool
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
				if err := store.NewRepoLocationStore(db.DB).UpsertLocal(context.Background(), "github", "acme", "api", localPath, true); err != nil {
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
			db := storetest.Open(t)
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

			// Act
			resp := callRepoLocationHandlerWithContext(t, ctx, handler, "ADD_REPO_BY_URL", tt.payload(root))
			data := responseData(t, resp)

			// Assert
			if data["ok"].(bool) != tt.wantOK {
				t.Fatalf("expected ok=%v, got %#v", tt.wantOK, data)
			}
			if tt.wantCloneBare != nil {
				calls := cloner.Calls()
				if len(calls) == 0 || calls[len(calls)-1].Bare != *tt.wantCloneBare {
					t.Fatalf("expected clone bare=%v, got calls=%#v", *tt.wantCloneBare, calls)
				}
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
