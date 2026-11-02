package commitviewed

import (
	"context"
	"database/sql"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
)

func insertPR(t *testing.T, db *sql.DB, prID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha)
		VALUES (?, 'github', 'Owner', 'Repo', 1, 'T', 'a', 'open',
		'2026-01-01', '2026-01-01', 'main', 'f', 'base', 'head')`, prID); err != nil {
		t.Fatalf("insert PR: %v", err)
	}
}

func TestCommitsViewed_MarkListUnmark_ScopedToLogin(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t).DB
	insertPR(t, db, "pr-1")
	s := New(db)
	if err := s.Mark(ctx, "pr-1", "me", "abc:0", 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark(ctx, "pr-1", "other", "abc:0", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Mark(ctx, "pr-1", "me", "abc:0", 3); err != nil {
		t.Fatal(err)
	}
	got, err := s.List(ctx, "pr-1", "me")
	if err != nil || len(got) != 1 || got["abc:0"] != 3 {
		t.Fatalf("List(me) = %v, %v; want {abc:0: 3}", got, err)
	}
	if err := s.Unmark(ctx, "pr-1", "me", "abc:0"); err != nil {
		t.Fatal(err)
	}
	got, err = s.List(ctx, "pr-1", "me")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("after Unmark = %v, %v; want non-nil empty map", got, err)
	}
	if err := s.Unmark(ctx, "pr-1", "me", "abc:0"); err != nil {
		t.Fatalf("unmark missing mark: %v", err)
	}
	got, err = s.List(ctx, "pr-1", "other")
	if err != nil || len(got) != 1 || got["abc:0"] != 1 {
		t.Fatalf("other login = %v, %v; want untouched", got, err)
	}
}
