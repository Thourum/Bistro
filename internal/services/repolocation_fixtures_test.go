package services

import (
	"context"
	"database/sql"
	"fmt"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/repolocation"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func testRepoLocationLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
