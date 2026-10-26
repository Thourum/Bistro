package store

import (
	"context"
	"strings"
	"testing"
)

func insertDismissalTestComment(t *testing.T, db *DB, prID, commentID string) {
	t.Helper()
	mustInsertPR(t, db, prID)
	mustExec(t, db, `INSERT INTO comments (id, pr_id, author, body, created_at, updated_at)
		VALUES (?, ?, 'alice', 'please fix', '2026-01-01', '2026-01-01')`, commentID, prID)
}

func TestDismissalsDismissIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	dismissals := NewDismissals(db.DB)
	ctx := context.Background()

	insertDismissalTestComment(t, db, "pr-dismiss", "comment-1")
	insertDismissalTestComment(t, db, "pr-dismiss", "comment-2")

	for _, commentIDs := range [][]string{{"comment-1", "comment-2"}, {"comment-1", "comment-2"}} {
		if err := dismissals.Dismiss(ctx, commentIDs, "github", "alice"); err != nil {
			t.Fatalf("Dismiss(%v) error: %v", commentIDs, err)
		}
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM comment_dismissals WHERE account_provider = 'github' AND account_login = 'alice'`).Scan(&count); err != nil {
		t.Fatalf("count dismissals: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected idempotent dismiss to store 2 rows, got %d", count)
	}
}

func TestDismissalsUndismiss(t *testing.T) {
	db := newTestDB(t)
	dismissals := NewDismissals(db.DB)
	ctx := context.Background()

	insertDismissalTestComment(t, db, "pr-undismiss", "comment-1")
	insertDismissalTestComment(t, db, "pr-undismiss", "comment-2")

	if err := dismissals.Dismiss(ctx, []string{"comment-1", "comment-2"}, "github", "alice"); err != nil {
		t.Fatalf("Dismiss error: %v", err)
	}
	if err := dismissals.Undismiss(ctx, []string{"comment-1"}, "github", "alice"); err != nil {
		t.Fatalf("Undismiss error: %v", err)
	}

	ids, err := dismissals.DismissedIDs(ctx, "pr-undismiss", "github", "alice")
	if err != nil {
		t.Fatalf("DismissedIDs error: %v", err)
	}
	if ids["comment-1"] {
		t.Fatal("comment-1 should have been undismissed")
	}
	if !ids["comment-2"] {
		t.Fatal("comment-2 should remain dismissed")
	}
}

func TestDismissalsDismissedIDsScopesByPRAndAccount(t *testing.T) {
	db := newTestDB(t)
	dismissals := NewDismissals(db.DB)
	ctx := context.Background()

	insertDismissalTestComment(t, db, "pr-one", "comment-1")
	insertDismissalTestComment(t, db, "pr-one", "comment-2")
	insertDismissalTestComment(t, db, "pr-two", "comment-3")

	for _, dismissal := range []struct {
		commentIDs []string
		provider   string
		login      string
	}{
		{commentIDs: []string{"comment-1"}, provider: "github", login: "alice"},
		{commentIDs: []string{"comment-2"}, provider: "github", login: "bob"},
		{commentIDs: []string{"comment-3"}, provider: "github", login: "alice"},
	} {
		if err := dismissals.Dismiss(ctx, dismissal.commentIDs, dismissal.provider, dismissal.login); err != nil {
			t.Fatalf("Dismiss(%v, %s/%s) error: %v", dismissal.commentIDs, dismissal.provider, dismissal.login, err)
		}
	}

	cases := []struct {
		name     string
		prID     string
		provider string
		login    string
		want     map[string]bool
	}{
		{name: "matching account and PR", prID: "pr-one", provider: "github", login: "alice", want: map[string]bool{"comment-1": true}},
		{name: "different login", prID: "pr-one", provider: "github", login: "bob", want: map[string]bool{"comment-2": true}},
		{name: "different PR", prID: "pr-two", provider: "github", login: "alice", want: map[string]bool{"comment-3": true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dismissals.DismissedIDs(ctx, tc.prID, tc.provider, tc.login)
			if err != nil {
				t.Fatalf("DismissedIDs error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("DismissedIDs length = %d, want %d (%v)", len(got), len(tc.want), got)
			}
			for id := range tc.want {
				if !got[id] {
					t.Fatalf("DismissedIDs missing %q from %v", id, got)
				}
			}
		})
	}
}

func TestDismissalsDismissUnknownCommentRollsBack(t *testing.T) {
	db := newTestDB(t)
	dismissals := NewDismissals(db.DB)
	ctx := context.Background()

	insertDismissalTestComment(t, db, "pr-rollback", "comment-1")

	err := dismissals.Dismiss(ctx, []string{"comment-1", "missing-comment"}, "github", "alice")
	if err == nil {
		t.Fatal("expected Dismiss to fail for an unknown comment id")
	}
	if !strings.Contains(err.Error(), "dismiss comments") {
		t.Fatalf("expected clear dismiss error, got %v", err)
	}

	ids, dismissedErr := dismissals.DismissedIDs(ctx, "pr-rollback", "github", "alice")
	if dismissedErr != nil {
		t.Fatalf("DismissedIDs error: %v", dismissedErr)
	}
	if len(ids) != 0 {
		t.Fatalf("expected failed Dismiss transaction to roll back all rows, got %v", ids)
	}
}
