package services

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/itterno/bistro/internal/store/repolocation"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestRepoLocationRemovePreflightAndRemove(t *testing.T) {
	database := storetest.Open(t)
	path := filepath.Join(t.TempDir(), "api")
	mustMkdirAll(path)
	insertRepoWithLocation(t, database, "github:acme/api", "github", "acme", "api", "cache", path, nil)
	insertRepoWithLocation(t, database, "github:acme/other", "github", "acme", "other", "local", t.TempDir(), nil)
	insertPRGraph(t, database, "pr1", "github:acme/api", "github", "acme", "api")
	insertPRGraph(t, database, "pr2", "github:acme/api", "github", "acme", "api")
	insertPRGraph(t, database, "pr3", "github:acme/other", "github", "acme", "other")
	service := &RepoLocation{DB: database.DB, Store: repolocation.New(database.DB), DeleteCacheDir: func(string) error { return nil }}
	ctx := context.Background()
	req := RepoRef{RepoID: "github:ACME/API"}
	counts, err := service.RemovePreflight(ctx, req)
	expected := RepoCounts{PRs: 2, Comments: 2, Threads: 2, Patchsets: 2, Actions: 2}
	if err != nil || counts != expected {
		t.Fatalf("preflight = %+v, %v", counts, err)
	}
	deleted, err := service.Remove(ctx, req)
	if err != nil || deleted != expected {
		t.Fatalf("remove = %+v, %v", deleted, err)
	}
	for _, table := range []string{"prs", "comments", "review_threads", "patchsets", "action_queue", "last_reviewed_sha", "files_viewed"} {
		assertRowCount(t, database.DB, "SELECT COUNT(*) FROM "+table, nil, 1)
	}
	assertRowCount(t, database.DB, "SELECT COUNT(*) FROM repos WHERE id=?", []any{"github:acme/api"}, 0)
}

func TestRepoLocationRemoveRollsBack(t *testing.T) {
	database := storetest.Open(t)
	insertRepoWithLocation(t, database, "github:acme/api", "github", "acme", "api", "local", t.TempDir(), nil)
	insertPRGraph(t, database, "pr1", "github:acme/api", "github", "acme", "api")
	// Fail after the earlier DELETEs have run, not during preflight counting.
	if _, err := database.Exec("DROP TABLE files_viewed"); err != nil {
		t.Fatal(err)
	}
	service := &RepoLocation{DB: database.DB, Store: repolocation.New(database.DB)}
	_, err := service.Remove(context.Background(), RepoRef{RepoID: "github:acme/api"})
	assertServiceCode(t, err, CodeInternal)
	for _, table := range []string{"repos", "prs", "comments", "review_threads", "patchsets", "action_queue", "last_reviewed_sha"} {
		assertRowCount(t, database.DB, "SELECT COUNT(*) FROM "+table, nil, 1)
	}
}
