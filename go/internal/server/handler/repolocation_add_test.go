package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	bistrogit "github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

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

func TestRepoLocationClearCacheOrphanSweepFailureKeepsPartialResults(t *testing.T) {
	// Arrange
	db := newRepoLocationMigratedDB(t)
	locations := store.NewRepoLocationStore(db.DB)
	cachePath := filepath.Join(t.TempDir(), "acme", "api")
	insertRepoWithLocation(t, db, "github:acme/api", "github", "acme", "api", "cache", cachePath, nil)
	deps := CacheMaintenanceDeps{
		Store:          locations,
		Locks:          bistrogit.NewRepoLocks(),
		DeleteCacheDir: func(string) error { return nil },
		SweepOrphans:   (&fakeRepoLocationSweeper{err: errors.New("sweep failed")}).SweepOrphans,
		Log:            testRepoLocationLogger(),
	}

	// Act
	resp := callRepoLocationHandler(t, NewClearCacheHandler(deps), "CLEAR_CACHE", map[string]any{})
	data := responseData(t, resp)

	// Assert: cache-row deletions that already succeeded are reported, and
	// the sweep failure surfaces as a failed entry instead of a bare error.
	if data["ok"].(bool) != true {
		t.Fatalf("expected ok=true partial result, got %#v", data)
	}
	removed := data["removed"].([]any)
	if len(removed) != 1 || removed[0] != "github:acme/api" {
		t.Fatalf("expected removed repo preserved, got %#v", data)
	}
	failed := data["failed"].([]any)
	if len(failed) != 1 {
		t.Fatalf("expected one failed entry for the sweep, got %#v", data)
	}
	failure := failed[0].(map[string]any)
	if failure["code"] != "delete_failed" || failure["message"] != "unable to sweep cache orphans" {
		t.Fatalf("unexpected sweep failure entry %#v", failure)
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
