package expertise

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestUpdaterRecomputesTouchedStaleFiles(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db.DB)
	repoPath := makeFixtureRepo(t)
	ctx := context.Background()
	indexNow := time.Date(2026, time.October, 10, 12, 0, 0, 0, time.UTC)
	git := NewGitCLI(newSilentExpertiseLogger())

	indexer := NewIndexer(newSilentExpertiseLogger(), store, git, func(context.Context) Identity {
		return Identity{Login: aliceName, AccountEmail: aliceEmail}
	})
	indexer.Now = func() time.Time { return indexNow }
	indexer.Workers = 2
	if err := indexer.Index(ctx, fixtureRepoID, repoPath); err != nil {
		t.Fatalf("Index: %v", err)
	}
	oldHead, err := git.HeadSHA(ctx, repoPath)
	if err != nil {
		t.Fatalf("old HeadSHA: %v", err)
	}

	initialMain := requireExpertiseRow(t, mustGetRows(t, store, ctx, "main.go"), aliceEmail)
	initialUtil := requireExpertiseRow(t, mustGetRows(t, store, ctx, "util.go"), bobEmail)
	if initialMain.ComputedAtSHA != oldHead || initialUtil.ComputedAtSHA != oldHead {
		t.Fatalf("initial SHAs main=%q util=%q, want old head %q", initialMain.ComputedAtSHA, initialUtil.ComputedAtSHA, oldHead)
	}
	if err := store.UpsertFileRows(ctx, []FileExpertiseRow{
		{RepoID: fixtureRepoID, FilePath: "generated.go", AuthorEmail: aliceEmail, SurvivingLines: 99, TotalLines: 100, Score: 99, ComputedAt: indexNow, ComputedAtSHA: oldHead},
		{RepoID: fixtureRepoID, FilePath: "vendor/dependency.go", AuthorEmail: aliceEmail, SurvivingLines: 99, TotalLines: 100, Score: 99, ComputedAt: indexNow, ComputedAtSHA: oldHead},
	}); err != nil {
		t.Fatalf("seed stale excluded/generated rows: %v", err)
	}

	appendFile(t, repoPath+"/main.go", "// alice followup 1\n")
	runGit(t, repoPath, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-10-11T10:00:00+00:00",
	}, "add", "main.go")
	runGit(t, repoPath, gitIdentity{
		name:  aliceName,
		email: aliceEmail,
		date:  "2026-10-11T10:00:00+00:00",
	}, "commit", "-m", "alice updates main")
	newHead, err := git.HeadSHA(ctx, repoPath)
	if err != nil {
		t.Fatalf("new HeadSHA: %v", err)
	}
	if newHead == oldHead {
		t.Fatal("new head matches old head after commit")
	}

	updater := NewUpdater(newSilentExpertiseLogger(), store, git, func(context.Context) Identity {
		return Identity{Login: aliceName, AccountEmail: aliceEmail}
	})
	updater.Now = func() time.Time { return time.Date(2026, time.October, 12, 12, 0, 0, 0, time.UTC) }

	if err := updater.Update(ctx, fixtureRepoID, repoPath, []string{"main.go", "util.go", "missing.go", "generated.go", "vendor/dependency.go"}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	updatedMainRows := mustGetRows(t, store, ctx, "main.go")
	updatedMain := requireExpertiseRow(t, updatedMainRows, aliceEmail)
	if updatedMain.ComputedAtSHA != newHead {
		t.Fatalf("main computed_at_sha = %q, want %q", updatedMain.ComputedAtSHA, newHead)
	}
	if updatedMain.SurvivingLines <= initialMain.SurvivingLines {
		t.Fatalf("main alice surviving lines = %d, want > initial %d", updatedMain.SurvivingLines, initialMain.SurvivingLines)
	}

	updatedUtilRows := mustGetRows(t, store, ctx, "util.go")
	updatedUtil := requireExpertiseRow(t, updatedUtilRows, bobEmail)
	if updatedUtil.ComputedAtSHA != newHead {
		t.Fatalf("util computed_at_sha = %q, want %q", updatedUtil.ComputedAtSHA, newHead)
	}
	if !updatedUtil.ComputedAt.After(initialUtil.ComputedAt) {
		t.Fatalf("util computed_at = %v, want after %v", updatedUtil.ComputedAt, initialUtil.ComputedAt)
	}

	missingRows := mustGetRows(t, store, ctx, "missing.go")
	if len(missingRows) != 0 {
		t.Fatalf("missing.go rows = %#v, want none", missingRows)
	}
	generatedRows := mustGetRows(t, store, ctx, "generated.go")
	if len(generatedRows) != 0 {
		t.Fatalf("generated.go rows = %#v, want deleted", generatedRows)
	}
	vendorRows := mustGetRows(t, store, ctx, "vendor/dependency.go")
	if len(vendorRows) != 0 {
		t.Fatalf("vendor/dependency.go rows = %#v, want deleted", vendorRows)
	}

	state, err := store.RepoState(ctx, fixtureRepoID)
	if err != nil {
		t.Fatalf("RepoState after update: %v", err)
	}
	if !state.IndexedSHA.Valid || state.IndexedSHA.String != newHead {
		t.Fatalf("state indexed_sha = %#v, want %q", state.IndexedSHA, newHead)
	}
	if state.Status != ExpertiseStatusReady {
		t.Fatalf("state status = %q, want ready", state.Status)
	}
}

func TestUpdaterNoOpsUnlessRepoReady(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db.DB)
	ctx := context.Background()
	git := newFakeIndexerGit(1)
	updater := NewUpdater(newSilentExpertiseLogger(), store, git, func(context.Context) Identity {
		return Identity{Login: aliceName, AccountEmail: aliceEmail}
	})

	if err := updater.Update(ctx, "github:owner/fresh", t.TempDir(), []string{"filea.go"}); err != nil {
		t.Fatalf("Update fresh repo: %v", err)
	}
	freshRows, err := store.GetFileExpertise(ctx, "github:owner/fresh", "filea.go")
	if err != nil {
		t.Fatalf("GetFileExpertise fresh repo: %v", err)
	}
	if len(freshRows) != 0 {
		t.Fatalf("fresh repo rows = %#v, want none", freshRows)
	}

	if err := store.SetRepoState(ctx, ExpertiseRepoState{RepoID: "github:owner/indexing", Enabled: true, Status: ExpertiseStatusIndexing}); err != nil {
		t.Fatalf("SetRepoState indexing: %v", err)
	}
	if err := updater.Update(ctx, "github:owner/indexing", t.TempDir(), []string{"filea.go"}); err != nil {
		t.Fatalf("Update indexing repo: %v", err)
	}
	indexingRows, err := store.GetFileExpertise(ctx, "github:owner/indexing", "filea.go")
	if err != nil {
		t.Fatalf("GetFileExpertise indexing repo: %v", err)
	}
	if len(indexingRows) != 0 {
		t.Fatalf("indexing repo rows = %#v, want none", indexingRows)
	}
}

func mustGetRows(t *testing.T, store *Store, ctx context.Context, filePath string) []FileExpertiseRow {
	t.Helper()
	rows, err := store.GetFileExpertise(ctx, fixtureRepoID, filePath)
	if err != nil {
		t.Fatalf("GetFileExpertise %s: %v", filePath, err)
	}
	return rows
}

var _ = sql.NullTime{}
