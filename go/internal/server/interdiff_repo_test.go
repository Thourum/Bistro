package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/itterno/bistro/internal/store"
)

func TestInterdiffRepoForPR(t *testing.T) {
	ctx := context.Background()
	db := openTestServerDB(t)
	locations := store.NewRepoLocationStore(db.DB)
	locator := prRepoLocator{db: db, locations: locations}
	const prID = "github:acme/api:1"

	assertLocation := func(wantURL, wantPath string, wantOK bool) {
		t.Helper()
		url, path, ok, err := locator.RepoForPR(ctx, prID)
		if err != nil || url != wantURL || path != wantPath || ok != wantOK {
			t.Fatalf("RepoForPR = (%q, %q, %v, %v), want (%q, %q, %v, nil)", url, path, ok, err, wantURL, wantPath, wantOK)
		}
	}
	assertLocation("", "", false)
	if _, err := db.ExecContext(ctx, `INSERT INTO prs
		(id, provider, owner, repo, number, title, author, state, created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github', 'Acme', 'API', 1, 'test', 'alice', 'open', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 'main', 'feature', 'base', 'head')`, prID); err != nil {
		t.Fatalf("insert PR: %v", err)
	}
	// GitHub keeps the owner's casing; repo-location IDs are lowercased.
	// Get must treat an absent repo row as unavailable, not a failed lookup.
	assertLocation("", "", false)
	insertServerRepo(t, db, "github:acme/api", "github", "acme", "api")
	assertLocation("", "", false)

	repoPath := filepath.Join(t.TempDir(), "repo")
	if err := locations.SetCache(ctx, "github:acme/api", repoPath); err != nil {
		t.Fatalf("set cache: %v", err)
	}
	assertLocation("https://github.com/Acme/API.git", repoPath, true)
	if err := locations.UpsertLocal(ctx, "github", "acme", "api", t.TempDir(), false); err != nil {
		t.Fatalf("set local clone: %v", err)
	}
	location, err := locations.Get(ctx, "github:acme/api")
	if err != nil || location == nil {
		t.Fatalf("get location = %v, err=%v", location, err)
	}
	assertLocation("https://github.com/Acme/API.git", location.LocalPath, true)

	if _, err := db.ExecContext(ctx, `DROP TABLE repos`); err != nil {
		t.Fatalf("drop repos: %v", err)
	}
	if _, _, ok, err := locator.RepoForPR(ctx, prID); ok || err == nil {
		t.Errorf("repo lookup failure: ok=%v err=%v, want error", ok, err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE prs`); err != nil {
		t.Fatalf("drop prs: %v", err)
	}
	if _, _, ok, err := locator.RepoForPR(ctx, prID); ok || err == nil {
		t.Errorf("PR lookup failure: ok=%v err=%v, want error", ok, err)
	}
}
