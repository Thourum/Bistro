package reviewdraft

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/itterno/bistro/internal/store/storetest"
)

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func TestReviewDrafts_CreateListUpdateDelete_ScopedToLogin(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t).DB
	drafts := New(db)

	single, err := drafts.Create(ctx, "me", Draft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 10, Body: "one", CommitSHA: "head"})
	if err != nil {
		t.Fatalf("create single: %v", err)
	}
	if single.ID == "" || single.StartLine != 0 || single.CreatedAt.IsZero() {
		t.Fatalf("single draft = %+v, want generated id, StartLine 0 and created time", single)
	}
	ranged, err := drafts.Create(ctx, "me", Draft{PRID: "pr-1", Path: "b.go", Side: "LEFT", StartLine: 3, Line: 5, Body: "range", CommitSHA: "old"})
	if err != nil {
		t.Fatalf("create range: %v", err)
	}
	if _, err := drafts.Create(ctx, "other", Draft{PRID: "pr-1", Path: "a.go", Side: "RIGHT", Line: 1, Body: "theirs", CommitSHA: "head"}); err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, err := drafts.Create(ctx, "me", Draft{PRID: "pr-2", Path: "a.go", Side: "RIGHT", Line: 1, Body: "another PR", CommitSHA: "head"}); err != nil {
		t.Fatalf("create another PR: %v", err)
	}
	// Second-resolution defaults and random ids do not imply insertion order.
	mustExec(t, db, `UPDATE review_drafts SET created_at = '2026-01-01T00:00:00Z' WHERE id = ?`, single.ID)
	mustExec(t, db, `UPDATE review_drafts SET created_at = '2026-01-02T00:00:00Z' WHERE id = ?`, ranged.ID)

	mine, err := drafts.List(ctx, "pr-1", "me")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mine) != 2 || mine[0].ID != single.ID || mine[1].ID != ranged.ID || mine[1].StartLine != 3 || mine[1].Side != "LEFT" || mine[1].CommitSHA != "old" {
		t.Fatalf("list = %+v, want my two drafts with the range intact", mine)
	}

	if _, err := drafts.UpdateBody(ctx, "other", single.ID, "hijack"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update by another login err = %v, want ErrNotFound", err)
	}
	if _, err := drafts.UpdateBody(ctx, "me", "missing", "edited"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing draft err = %v, want ErrNotFound", err)
	}
	updated, err := drafts.UpdateBody(ctx, "me", single.ID, "edited")
	if err != nil || updated.Body != "edited" || updated.ID != single.ID || updated.CommitSHA != "head" {
		t.Fatalf("update = %+v, %v", updated, err)
	}

	if _, err := drafts.Delete(ctx, "other", []string{ranged.ID}); err != nil {
		t.Fatalf("delete by other: %v", err)
	}
	if _, err := drafts.Delete(ctx, "me", []string{single.ID, "missing"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	left, err := drafts.List(ctx, "pr-1", "me")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(left) != 1 || left[0].ID != ranged.ID {
		t.Fatalf("after delete = %+v, want only the range draft", left)
	}
	if _, err := drafts.Delete(ctx, "me", []string{ranged.ID}); err != nil {
		t.Fatalf("delete last draft: %v", err)
	}
	empty, err := drafts.List(ctx, "pr-1", "me")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %+v, %v, want non-nil empty slice", empty, err)
	}
	if _, err := drafts.Delete(ctx, "me", nil); err != nil {
		t.Fatalf("delete no drafts: %v", err)
	}
}

func TestReviewDrafts_RejectsInvalidRange(t *testing.T) {
	drafts := New(storetest.Open(t).DB)
	for _, test := range []struct {
		name      string
		side      string
		startLine int
		line      int
	}{
		{name: "reversed range", side: "RIGHT", startLine: 9, line: 5},
		{name: "equal endpoints", side: "RIGHT", startLine: 5, line: 5},
		{name: "negative start", side: "RIGHT", startLine: -1, line: 5},
		{name: "zero line", side: "RIGHT", line: 0},
		{name: "negative line", side: "RIGHT", line: -1},
		{name: "invalid side", side: "CENTER", line: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := drafts.Create(context.Background(), "me", Draft{PRID: "pr-1", Path: "a.go", Side: test.side, StartLine: test.startLine, Line: test.line, Body: "x", CommitSHA: "head"})
			if err == nil {
				t.Fatal("expected the database constraints to reject the draft")
			}
		})
	}
}

func TestReviewDrafts_ListOrdersByCreatedAtAndID(t *testing.T) {
	db := storetest.Open(t).DB
	const earlier = "2026-01-01T00:00:00Z"
	const later = "2026-01-02T00:00:00Z"
	for _, row := range []struct{ id, createdAt string }{{"b", later}, {"a", later}, {"c", earlier}} {
		mustExec(t, db, `INSERT INTO review_drafts (id, pr_id, login, path, side, line, body, commit_sha, created_at)
			VALUES (?, 'pr-1', 'me', 'a.go', 'RIGHT', 1, 'x', 'head', ?)`, row.id, row.createdAt)
	}
	drafts, err := New(db).List(context.Background(), "pr-1", "me")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(drafts) != 3 || drafts[0].ID != "c" || drafts[1].ID != "a" || drafts[2].ID != "b" {
		t.Fatalf("list = %+v, want ids c, a, b", drafts)
	}
}

func TestReviewDrafts_DeleteRollsBackOnFailure(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t).DB
	drafts := New(db)
	for _, id := range []string{"first", "second"} {
		mustExec(t, db, `INSERT INTO review_drafts (id, pr_id, login, path, side, line, body, commit_sha)
			VALUES (?, 'pr-1', 'me', 'a.go', 'RIGHT', 1, 'x', 'head')`, id)
	}
	mustExec(t, db, `CREATE TRIGGER fail_second_delete BEFORE DELETE ON review_drafts
		WHEN OLD.id = 'second' BEGIN SELECT RAISE(ABORT, 'blocked delete'); END`)
	if deleted, err := drafts.Delete(ctx, "me", []string{"first", "second"}); err == nil || deleted != 0 {
		t.Fatalf("delete = %d, %v; want zero count and failure", deleted, err)
	}
	remaining, err := drafts.List(ctx, "pr-1", "me")
	if err != nil {
		t.Fatalf("list after failed delete: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("after failed delete = %+v, want both drafts preserved", remaining)
	}
}
