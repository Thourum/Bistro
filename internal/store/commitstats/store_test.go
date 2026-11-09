package commitstats

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
)

const (
	originalVersion = 1
	updatedVersion  = 2
)

func insertPR(t *testing.T, db *sql.DB, prID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github', 'Owner', ?, 1, 'T', 'a', 'open',
		'2026-01-01', '2026-01-01', 'main', 'f', 'base', 'head')`, prID, prID); err != nil {
		t.Fatalf("insert PR: %v", err)
	}
}

func TestCommitStats_ListFiltersVersionAndReplacePrunes(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t).DB
	insertPR(t, db, "pr-1")
	s := New(db)
	original := []Stats{
		{SHA: "a", Additions: 1, Deletions: 2, FileCount: 1, PatchID: "patch-a"},
		{SHA: "b", Additions: 2, Deletions: 1, FileCount: 2, PatchID: "patch-b"},
	}
	if err := s.Upsert(ctx, "pr-1", originalVersion, original); err != nil {
		t.Fatal(err)
	}
	newer := Stats{SHA: "c", Additions: 3, Deletions: 4, FileCount: 2, PatchID: "patch-c"}
	if err := s.Upsert(ctx, "pr-1", updatedVersion, []Stats{newer}); err != nil {
		t.Fatal(err)
	}
	assertStats(t, s, "pr-1", originalVersion, map[string]Stats{"a": original[0], "b": original[1]})
	assertStats(t, s, "pr-1", updatedVersion, map[string]Stats{"c": newer})

	replacement := Stats{SHA: "b", Additions: 5, Deletions: 6, FileCount: 3, PatchID: "updated-b"}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := ReplaceTx(ctx, tx, "pr-1", originalVersion, []Stats{replacement}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertStats(t, s, "pr-1", originalVersion, map[string]Stats{"b": replacement})
	assertStats(t, s, "pr-1", updatedVersion, map[string]Stats{})
	assertStats(t, s, "no-rows", originalVersion, map[string]Stats{})
}

func TestCommitStats_UpsertUpdatesVersionAndPreservesOtherRows(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t).DB
	insertPR(t, db, "pr-1")
	insertPR(t, db, "pr-2")
	s := New(db)
	original := Stats{SHA: "a", Additions: 1, Deletions: 2, FileCount: 1, PatchID: "patch-a"}
	preserved := Stats{SHA: "b", Additions: 2, Deletions: 1, FileCount: 2, PatchID: "patch-b"}
	if err := s.Upsert(ctx, "pr-1", originalVersion, []Stats{original, preserved}); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(ctx, "pr-2", originalVersion, []Stats{original}); err != nil {
		t.Fatal(err)
	}
	updated := Stats{SHA: "a", Additions: 3, Deletions: 4, FileCount: 2, PatchID: "updated-a"}
	if err := s.Upsert(ctx, "pr-1", updatedVersion, []Stats{updated}); err != nil {
		t.Fatal(err)
	}
	assertStats(t, s, "pr-1", originalVersion, map[string]Stats{"b": preserved})
	assertStats(t, s, "pr-1", updatedVersion, map[string]Stats{"a": updated})
	assertStats(t, s, "pr-2", originalVersion, map[string]Stats{"a": original})
}

func TestCommitStats_UpsertRollsBackBatchOnFailure(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t).DB
	insertPR(t, db, "pr-1")
	s := New(db)
	original := Stats{SHA: "a", Additions: 1, PatchID: "patch-a"}
	if err := s.Upsert(ctx, "pr-1", originalVersion, []Stats{original}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_commit_stats BEFORE INSERT ON commit_stats
		WHEN NEW.sha = 'rejected' BEGIN SELECT RAISE(ABORT, 'rejected commit'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert(ctx, "pr-1", updatedVersion, []Stats{
		{SHA: "a", Additions: 2, PatchID: "updated-a"},
		{SHA: "rejected"},
	}); err == nil {
		t.Fatal("Upsert succeeded despite rejected second row")
	}
	assertStats(t, s, "pr-1", originalVersion, map[string]Stats{"a": original})
	assertStats(t, s, "pr-1", updatedVersion, map[string]Stats{})
}

func TestCommitStats_ReplaceTxRollbackAndEmptyRange(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t).DB
	insertPR(t, db, "pr-1")
	insertPR(t, db, "pr-2")
	s := New(db)
	original := Stats{SHA: "a", Additions: 1, PatchID: "patch-a"}
	for _, prID := range []string{"pr-1", "pr-2"} {
		if err := s.Upsert(ctx, prID, originalVersion, []Stats{original}); err != nil {
			t.Fatal(err)
		}
	}
	for _, commit := range []bool{false, true} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := ReplaceTx(ctx, tx, "pr-1", updatedVersion, nil); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if !commit {
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			assertStats(t, s, "pr-1", originalVersion, map[string]Stats{"a": original})
			continue
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		assertStats(t, s, "pr-1", originalVersion, map[string]Stats{})
	}
	assertStats(t, s, "pr-2", originalVersion, map[string]Stats{"a": original})
}

func assertStats(t *testing.T, s *Store, prID string, version int, want map[string]Stats) {
	t.Helper()
	got, err := s.List(context.Background(), prID, version)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("List(%s, %d) = %v, %v; want %v", prID, version, got, err, want)
	}
}
