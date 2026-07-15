package store

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

func TestMigration026AddsLocationColumns(t *testing.T) {
	db := newTestDBMigratedThrough(t, 25)

	// Existing row survives migration 026 and new columns are readable with NULL defaults.
	mustExec(t, db, `INSERT INTO repos (id, provider, owner, repo) VALUES ('github:acme/api','github','acme','api')`)

	if err := NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("run remaining migrations: %v", err)
	}

	var localPath, source, lastUsed, status, code, msg any
	err := db.QueryRow(`SELECT local_path, source, last_used_at, location_status, location_error_code, location_error FROM repos WHERE id='github:acme/api'`).
		Scan(&localPath, &source, &lastUsed, &status, &code, &msg)
	if err != nil {
		t.Fatalf("select new columns: %v", err)
	}
	if localPath != nil || source != nil || lastUsed != nil || status != nil || code != nil || msg != nil {
		t.Fatalf(
			"expected NULL defaults, got local_path=%v source=%v last_used_at=%v location_status=%v location_error_code=%v location_error=%v",
			localPath,
			source,
			lastUsed,
			status,
			code,
			msg,
		)
	}
}

func TestRepoLocationStoreUpsertLocalCreatesWatchedRepo(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)
	path := t.TempDir()

	// Act
	err := store.UpsertLocal(ctx, "GitHub", "Acme", "API", path)

	// Assert
	if err != nil {
		t.Fatalf("upsert local repo: %v", err)
	}
	location := mustGetRepoLocation(t, store, "github:acme/api")
	if location.RepoID != "github:acme/api" {
		t.Fatalf("expected lowercased repo id, got %q", location.RepoID)
	}
	if location.Provider != "github" || location.Owner != "acme" || location.Repo != "api" {
		t.Fatalf("expected lowercased provider/owner/repo, got %q/%q/%q", location.Provider, location.Owner, location.Repo)
	}
	if location.LocalPath != path {
		t.Fatalf("expected local path %q, got %q", path, location.LocalPath)
	}
	if location.Source != "local" {
		t.Fatalf("expected source local, got %q", location.Source)
	}
	if location.LocationStatus != "ok" {
		t.Fatalf("expected status ok, got %q", location.LocationStatus)
	}
	assertWatched(t, db, "github:acme/api", true)
	assertRecentTime(t, location.LastUsedAt)
}

func TestRepoLocationStoreUpsertLocalUpdatesUnhealthyPath(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)
	oldPath := t.TempDir()
	newPath := t.TempDir()
	mustExec(t, db, `
		INSERT INTO repos (id, provider, owner, repo, watched, local_path, source, location_status, location_error_code, location_error)
		VALUES ('github:acme/api', 'github', 'acme', 'api', 0, ?, 'local', 'missing', 'not_found', 'gone')
	`, oldPath)

	// Act
	err := store.UpsertLocal(ctx, "github", "acme", "api", newPath)

	// Assert
	if err != nil {
		t.Fatalf("upsert local repo: %v", err)
	}
	location := mustGetRepoLocation(t, store, "github:acme/api")
	if location.LocalPath != newPath {
		t.Fatalf("expected unhealthy path updated to %q, got %q", newPath, location.LocalPath)
	}
	if location.Source != "local" || location.LocationStatus != "ok" {
		t.Fatalf("expected healthy local location, got source=%q status=%q", location.Source, location.LocationStatus)
	}
	if location.LocationErrorCode != "" || location.LocationError != "" {
		t.Fatalf("expected cleared errors, got code=%q msg=%q", location.LocationErrorCode, location.LocationError)
	}
	assertWatched(t, db, "github:acme/api", true)
}

func TestRepoLocationStoreUpsertLocalPreservesExistingHealthyPathThatExists(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)
	existingPath := t.TempDir()
	duplicatePath := t.TempDir()
	mustExec(t, db, `
		INSERT INTO repos (id, provider, owner, repo, watched, local_path, source, location_status)
		VALUES ('github:acme/api', 'github', 'acme', 'api', 1, ?, 'local', 'ok')
	`, existingPath)

	// Act
	err := store.UpsertLocal(ctx, "github", "acme", "api", duplicatePath)

	// Assert
	if err != nil {
		t.Fatalf("upsert local repo: %v", err)
	}
	location := mustGetRepoLocation(t, store, "github:acme/api")
	if location.LocalPath != existingPath {
		t.Fatalf("expected existing healthy path %q to win, got %q", existingPath, location.LocalPath)
	}
	if location.Source != "local" || location.LocationStatus != "ok" {
		t.Fatalf("expected healthy local location, got source=%q status=%q", location.Source, location.LocationStatus)
	}
}

func TestRepoLocationStoreSetCacheMarksRepoAsCacheCloned(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)
	mustInsertRepo(t, db, "github:acme/api")
	path := t.TempDir()

	// Act
	err := store.SetCache(ctx, "github:acme/api", path)

	// Assert
	if err != nil {
		t.Fatalf("set cache location: %v", err)
	}
	location := mustGetRepoLocation(t, store, "github:acme/api")
	if location.LocalPath != path {
		t.Fatalf("expected cache path %q, got %q", path, location.LocalPath)
	}
	if location.Source != "cache" {
		t.Fatalf("expected source cache, got %q", location.Source)
	}
	if location.LocationStatus != "ok" {
		t.Fatalf("expected status ok, got %q", location.LocationStatus)
	}
	assertRecentTime(t, location.LastUsedAt)
}

func TestRepoLocationStoreTouchSetsLastUsedAt(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)
	mustInsertRepo(t, db, "github:acme/api")

	// Act
	err := store.Touch(ctx, "github:acme/api")

	// Assert
	if err != nil {
		t.Fatalf("touch repo location: %v", err)
	}
	location := mustGetRepoLocation(t, store, "github:acme/api")
	assertRecentTime(t, location.LastUsedAt)
}

func TestRepoLocationStoreMarkDegradedAndClearDegradedRoundTrip(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)
	path := t.TempDir()
	mustExec(t, db, `
		INSERT INTO repos (id, provider, owner, repo, local_path, source, location_status)
		VALUES ('github:acme/api', 'github', 'acme', 'api', ?, 'local', 'ok')
	`, path)

	// Act
	markErr := store.MarkDegraded(ctx, "github:acme/api", "missing", "not_found", "repo missing")
	marked := mustGetRepoLocation(t, store, "github:acme/api")
	clearErr := store.ClearDegraded(ctx, "github:acme/api")
	cleared := mustGetRepoLocation(t, store, "github:acme/api")

	// Assert
	if markErr != nil {
		t.Fatalf("mark degraded: %v", markErr)
	}
	if clearErr != nil {
		t.Fatalf("clear degraded: %v", clearErr)
	}
	if marked.LocalPath != path {
		t.Fatalf("expected degraded location to keep path %q, got %q", path, marked.LocalPath)
	}
	if marked.LocationStatus != "missing" || marked.LocationErrorCode != "not_found" || marked.LocationError != "repo missing" {
		t.Fatalf("unexpected degraded fields: status=%q code=%q msg=%q", marked.LocationStatus, marked.LocationErrorCode, marked.LocationError)
	}
	if cleared.LocalPath != path {
		t.Fatalf("expected clear degraded to keep path %q, got %q", path, cleared.LocalPath)
	}
	if cleared.LocationStatus != "ok" || cleared.LocationErrorCode != "" || cleared.LocationError != "" {
		t.Fatalf("unexpected cleared fields: status=%q code=%q msg=%q", cleared.LocationStatus, cleared.LocationErrorCode, cleared.LocationError)
	}
}

func TestRepoLocationStoreClearCacheLocationNullsLocationFields(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)
	mustExec(t, db, `
		INSERT INTO repos (id, provider, owner, repo, local_path, source, last_used_at, location_status, location_error_code, location_error)
		VALUES ('github:acme/api', 'github', 'acme', 'api', '/tmp/cache', 'cache', '2026-01-01T00:00:00Z', 'missing', 'gone', 'not found')
	`)

	// Act
	err := store.ClearCacheLocation(ctx, "github:acme/api")

	// Assert
	if err != nil {
		t.Fatalf("clear cache location: %v", err)
	}
	location := mustGetRepoLocation(t, store, "github:acme/api")
	if location.LocalPath != "" || location.Source != "" || location.LastUsedAt != nil || location.LocationStatus != "" || location.LocationErrorCode != "" || location.LocationError != "" {
		t.Fatalf("expected location fields cleared, got %+v", location)
	}
}

func TestRepoLocationStoreGetReturnsNilWhenAbsent(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)

	// Act
	location, err := store.Get(ctx, "github:acme/api")

	// Assert
	if err != nil {
		t.Fatalf("get missing repo location: %v", err)
	}
	if location != nil {
		t.Fatalf("expected nil location, got %+v", location)
	}
}

func TestRepoLocationStoreListBySourceFiltersRows(t *testing.T) {
	// Arrange
	ctx := context.Background()
	db := newTestDBMigratedThrough(t, 26)
	store := NewRepoLocationStore(db.DB)
	mustExec(t, db, `
		INSERT INTO repos (id, provider, owner, repo, local_path, source, location_status)
		VALUES
			('github:acme/api', 'github', 'acme', 'api', '/tmp/local', 'local', 'ok'),
			('github:acme/web', 'github', 'acme', 'web', '/tmp/cache', 'cache', 'ok'),
			('github:acme/cli', 'github', 'acme', 'cli', NULL, NULL, NULL)
	`)

	// Act
	locations, err := store.ListBySource(ctx, "local")

	// Assert
	if err != nil {
		t.Fatalf("list repo locations by source: %v", err)
	}
	if len(locations) != 1 {
		t.Fatalf("expected 1 local location, got %d: %+v", len(locations), locations)
	}
	if locations[0].RepoID != "github:acme/api" || locations[0].Source != "local" {
		t.Fatalf("unexpected local location: %+v", locations[0])
	}
}

func newTestDBMigratedThrough(t *testing.T, throughVersion int) *DB {
	t.Helper()

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	provider, err := goose.NewProvider(goose.DialectSQLite3, db.DB, mustSubMigrationsFS(t))
	if err != nil {
		t.Fatalf("create goose provider: %v", err)
	}
	if _, err := provider.UpTo(ctx, int64(throughVersion)); err != nil {
		t.Fatalf("apply migrations through %d: %v", throughVersion, err)
	}

	return db
}

func mustSubMigrationsFS(t *testing.T) fs.FS {
	t.Helper()
	migrationFS, err := fs.Sub(migrationsFS, migrationsDir)
	if err != nil {
		t.Fatalf("sub migrations fs: %v", err)
	}
	return migrationFS
}

func mustInsertRepo(t *testing.T, db *DB, repoID string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO repos (id, provider, owner, repo) VALUES (?, 'github', 'acme', 'api')`, repoID)
}

func mustGetRepoLocation(t *testing.T, store *RepoLocationStore, repoID string) *RepoLocation {
	t.Helper()
	location, err := store.Get(context.Background(), repoID)
	if err != nil {
		t.Fatalf("get repo location %s: %v", repoID, err)
	}
	if location == nil {
		t.Fatalf("expected repo location %s", repoID)
	}
	return location
}

func assertWatched(t *testing.T, db *DB, repoID string, expected bool) {
	t.Helper()
	var watched int
	if err := db.QueryRow(`SELECT watched FROM repos WHERE id = ?`, repoID).Scan(&watched); err != nil {
		t.Fatalf("select watched: %v", err)
	}
	if (watched != 0) != expected {
		t.Fatalf("expected watched=%v, got %d", expected, watched)
	}
}

func assertRecentTime(t *testing.T, got *time.Time) {
	t.Helper()
	if got == nil {
		t.Fatal("expected timestamp, got nil")
	}
	if time.Since(*got) > time.Minute {
		t.Fatalf("expected recent timestamp, got %s", got.Format(time.RFC3339Nano))
	}
	if time.Until(*got) > time.Minute {
		t.Fatalf("expected timestamp not far in future, got %s", got.Format(time.RFC3339Nano))
	}
}
