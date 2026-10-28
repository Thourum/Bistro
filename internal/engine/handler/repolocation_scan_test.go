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
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/storetest"
	"github.com/itterno/bistro/internal/ws"
	_ "github.com/mattn/go-sqlite3"
)

func TestRepoLocationRecoverRepoPathAndToCache(t *testing.T) {
	// Arrange
	root := t.TempDir()
	db := storetest.Open(t)
	store := repolocation.New(db.DB)
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

func assertLocationStatus(t *testing.T, store *repolocation.Store, repoID, wantStatus, wantCode string) {
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

func TestTriggerSyncRequestsAllRepos(t *testing.T) {
	triggers := make(chan string, 1)
	triggerSync(triggers, testRepoLocationLogger())
	select {
	case repoID := <-triggers:
		if repoID != "" {
			t.Fatalf("got repo trigger %q, want full sync", repoID)
		}
	default:
		t.Fatal("missing sync trigger")
	}
	triggers <- "already queued"
	triggerSync(triggers, testRepoLocationLogger()) // Must not block a full queue.
	triggerSync(nil, testRepoLocationLogger())
}

func testRepoLocationLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
