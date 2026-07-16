package server

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	bistrogit "github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/store"
)

const repoLocationResolverURL = "https://github.com/acme/api.git"

func TestConfigureRepoLocationStoreWiresGitClientLocalResolver(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newServerRepoLocationMigratedDB(t)
	locations := store.NewRepoLocationStore(db.DB)
	missingPath := filepath.Join(t.TempDir(), "missing")
	if err := locations.UpsertLocal(ctx, "github", "acme", "api", missingPath, false); err != nil {
		t.Fatalf("upsert local repo: %v", err)
	}

	client := bistrogit.NewClient(bistrogit.NewCache())
	app := &App{log: testRepoLocationResolverLogger(), gitClient: client}

	// Act
	app.configureRepoLocationStore(locations)
	result, err := client.Clone(ctx, repoLocationResolverURL, "base", "head")

	// Assert
	if result != nil {
		t.Fatalf("expected no clone result for degraded local repo, got %#v", result)
	}
	var locationErr *bistrogit.LocationError
	if !errors.As(err, &locationErr) {
		t.Fatalf("expected LocationError from wired resolver, got %T: %v", err, err)
	}
	if locationErr.Code != "path_missing" {
		t.Fatalf("expected path_missing, got %q", locationErr.Code)
	}
	location := mustGetServerRepoLocation(t, locations, "github:acme/api")
	if location.LocationStatus != "missing" || location.LocationErrorCode != "path_missing" {
		t.Fatalf("expected missing/path_missing persisted, got %+v", location)
	}
}

func TestRepoLocationResolverPersistsGitClientCallbacks(t *testing.T) {
	// Arrange
	db := newServerRepoLocationMigratedDB(t)
	locations := store.NewRepoLocationStore(db.DB)
	insertServerRepo(t, db, "github:acme/api", "github", "acme", "api")
	resolver := newRepoLocationResolver(locations, testRepoLocationResolverLogger())
	cachePath := filepath.Join(t.TempDir(), "cache", "acme", "api")

	// Act
	resolver.OnCacheCloned(repoLocationResolverURL, cachePath)
	resolver.OnUsed(repoLocationResolverURL)
	resolver.OnLocalDegraded(repoLocationResolverURL, "remote_mismatch", "remote_mismatch", "remote mismatch")

	// Assert
	location := mustGetServerRepoLocation(t, locations, "github:acme/api")
	if location.Source != "cache" || location.LocalPath != cachePath {
		t.Fatalf("expected cache source/path persisted, got %+v", location)
	}
	if location.LastUsedAt == nil {
		t.Fatal("expected last_used_at to be set")
	}
	if location.LocationStatus != "remote_mismatch" || location.LocationErrorCode != "remote_mismatch" || location.LocationError != "remote mismatch" {
		t.Fatalf("expected degraded state persisted, got %+v", location)
	}
}

func newServerRepoLocationMigratedDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func insertServerRepo(t *testing.T, db *store.DB, repoID, provider, owner, repo string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO repos (id, provider, owner, repo, watched) VALUES (?, ?, ?, ?, 1)`, repoID, provider, owner, repo)
	if err != nil {
		t.Fatalf("insert repo: %v", err)
	}
}

func mustGetServerRepoLocation(t *testing.T, locations *store.RepoLocationStore, repoID string) *store.RepoLocation {
	t.Helper()
	location, err := locations.Get(context.Background(), repoID)
	if err != nil {
		t.Fatalf("get repo location: %v", err)
	}
	if location == nil {
		t.Fatalf("expected repo location %s", repoID)
	}
	return location
}

func testRepoLocationResolverLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
