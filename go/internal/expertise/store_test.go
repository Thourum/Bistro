package expertise

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	storepkg "github.com/itterno/bistro/internal/store"
)

func newTestDB(t *testing.T) *storepkg.DB {
	t.Helper()
	db, err := storepkg.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("newTestDB open: %v", err)
	}
	if err := storepkg.NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		t.Fatalf("newTestDB migrate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStoreFileExpertiseRepoStateAndReviewedPRCount(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db.DB)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	lastCommit := now.Add(-24 * time.Hour)

	err := store.UpsertFileRows(ctx, []FileExpertiseRow{
		{
			RepoID:         "github:owner/repo",
			FilePath:       "main.go",
			AuthorEmail:    "alice@example.com",
			AuthorLogin:    sql.NullString{String: "alice", Valid: true},
			SurvivingLines: 12,
			TotalLines:     20,
			CommitScore:    0.2,
			IsOriginal:     true,
			Score:          55,
			LastCommitAt:   sql.NullTime{Time: lastCommit, Valid: true},
			Dormant:        false,
			ComputedAt:     now,
			ComputedAtSHA:  "sha-one",
		},
		{
			RepoID:         "github:owner/repo",
			FilePath:       "main.go",
			AuthorEmail:    "bob@example.com",
			SurvivingLines: 18,
			TotalLines:     20,
			CommitScore:    0.8,
			IsOriginal:     false,
			Score:          72,
			Dormant:        true,
			ComputedAt:     now,
			ComputedAtSHA:  "sha-one",
		},
	})
	if err != nil {
		t.Fatalf("UpsertFileRows: %v", err)
	}

	rows, err := store.GetFileExpertise(ctx, "github:owner/repo", "main.go")
	if err != nil {
		t.Fatalf("GetFileExpertise: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("GetFileExpertise returned %d rows, want 2", len(rows))
	}
	if rows[0].AuthorEmail != "bob@example.com" || rows[1].AuthorEmail != "alice@example.com" {
		t.Fatalf("GetFileExpertise order = [%s, %s], want score DESC", rows[0].AuthorEmail, rows[1].AuthorEmail)
	}

	state := ExpertiseRepoState{
		RepoID:     "github:owner/repo",
		Enabled:    true,
		Status:     "indexing",
		IndexedSHA: sql.NullString{String: "sha-one", Valid: true},
		IndexedAt:  sql.NullTime{Time: now, Valid: true},
		FilesTotal: sql.NullInt64{Int64: 10, Valid: true},
		FilesDone:  sql.NullInt64{Int64: 3, Valid: true},
	}
	if err := store.SetRepoState(ctx, state); err != nil {
		t.Fatalf("SetRepoState indexing: %v", err)
	}
	state.Status = "ready"
	state.FilesDone = sql.NullInt64{Int64: 10, Valid: true}
	if err := store.SetRepoState(ctx, state); err != nil {
		t.Fatalf("SetRepoState ready: %v", err)
	}

	gotState, err := store.RepoState(ctx, "github:owner/repo")
	if err != nil {
		t.Fatalf("RepoState: %v", err)
	}
	if gotState.Status != "ready" || gotState.FilesDone.Int64 != 10 {
		t.Fatalf("RepoState = status %q files_done %d, want ready/10", gotState.Status, gotState.FilesDone.Int64)
	}

	count, err := store.ReviewedPRCount(ctx, "github:owner/repo", "github", "alice", "alice")
	if err != nil {
		t.Fatalf("ReviewedPRCount: %v", err)
	}
	if count != 0 {
		t.Fatalf("ReviewedPRCount = %d, want 0", count)
	}
}

func TestStoreReplaceFileRowsAndSettings(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db.DB)
	ctx := context.Background()
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	initial := FileExpertiseRow{
		RepoID:         "github:owner/repo",
		FilePath:       "internal/store.go",
		AuthorEmail:    "alice@example.com",
		SurvivingLines: 10,
		TotalLines:     10,
		Score:          70,
		ComputedAt:     now,
		ComputedAtSHA:  "sha-one",
	}
	if err := store.UpsertFileRows(ctx, []FileExpertiseRow{initial}); err != nil {
		t.Fatalf("UpsertFileRows initial: %v", err)
	}

	replacement := FileExpertiseRow{
		RepoID:         "github:owner/repo",
		FilePath:       "internal/store.go",
		AuthorEmail:    "bob@example.com",
		SurvivingLines: 9,
		TotalLines:     10,
		Score:          80,
		ComputedAt:     now,
		ComputedAtSHA:  "sha-two",
	}
	if err := store.ReplaceFileRows(ctx, "github:owner/repo", "internal/store.go", []FileExpertiseRow{replacement}); err != nil {
		t.Fatalf("ReplaceFileRows: %v", err)
	}

	rows, err := store.GetFileExpertise(ctx, "github:owner/repo", "internal/store.go")
	if err != nil {
		t.Fatalf("GetFileExpertise after replace: %v", err)
	}
	if len(rows) != 1 || rows[0].AuthorEmail != "bob@example.com" {
		t.Fatalf("rows after replace = %#v, want only bob", rows)
	}

	aliases := []string{"alice@example.com", "alice@users.noreply.github.com"}
	if err := store.SetSetting(ctx, "alias_emails", aliases); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	var gotAliases []string
	if err := store.Setting(ctx, "alias_emails", &gotAliases); err != nil {
		t.Fatalf("Setting: %v", err)
	}
	if len(gotAliases) != 2 || gotAliases[0] != aliases[0] || gotAliases[1] != aliases[1] {
		t.Fatalf("Setting aliases = %#v, want %#v", gotAliases, aliases)
	}

	var missing []string
	if err := store.Setting(ctx, "missing", &missing); !errors.Is(err, ErrSettingNotFound) {
		t.Fatalf("Setting missing error = %v, want ErrSettingNotFound", err)
	}
}
