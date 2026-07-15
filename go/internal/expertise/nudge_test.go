package expertise

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestShouldNudge(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		setup func(t *testing.T, db *sql.DB, store *Store)
		want  bool
	}{
		{
			name: "true after ten reviewed local repo PRs",
			setup: func(t *testing.T, db *sql.DB, store *Store) {
				insertNudgeRepo(t, db, fixtureRepoID, "local")
				insertReviewedPRs(t, db, fixtureRepoID, "github", "alice", "alice", 10)
			},
			want: true,
		},
		{
			name: "false when enabled",
			setup: func(t *testing.T, db *sql.DB, store *Store) {
				insertNudgeRepo(t, db, fixtureRepoID, "local")
				insertReviewedPRs(t, db, fixtureRepoID, "github", "alice", "alice", 10)
				setNudgeState(t, store, ExpertiseRepoState{RepoID: fixtureRepoID, Enabled: true, Status: ExpertiseStatusReady})
			},
			want: false,
		},
		{
			name: "false when dismissed",
			setup: func(t *testing.T, db *sql.DB, store *Store) {
				insertNudgeRepo(t, db, fixtureRepoID, "local")
				insertReviewedPRs(t, db, fixtureRepoID, "github", "alice", "alice", 10)
				setNudgeState(t, store, ExpertiseRepoState{
					RepoID:           fixtureRepoID,
					Enabled:          false,
					Status:           ExpertiseStatusIdle,
					NudgeDismissedAt: sql.NullTime{Time: time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC), Valid: true},
				})
			},
			want: false,
		},
		{
			name: "false below threshold",
			setup: func(t *testing.T, db *sql.DB, store *Store) {
				insertNudgeRepo(t, db, fixtureRepoID, "local")
				insertReviewedPRs(t, db, fixtureRepoID, "github", "alice", "alice", 9)
			},
			want: false,
		},
		{
			name: "false for cache repo",
			setup: func(t *testing.T, db *sql.DB, store *Store) {
				insertNudgeRepo(t, db, fixtureRepoID, "cache")
				insertReviewedPRs(t, db, fixtureRepoID, "github", "alice", "alice", 10)
			},
			want: false,
		},
		{
			name: "false when rows belong to a different account",
			setup: func(t *testing.T, db *sql.DB, store *Store) {
				insertNudgeRepo(t, db, fixtureRepoID, "local")
				insertReviewedPRs(t, db, fixtureRepoID, "github", "bob", "alice", 10)
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			store := NewStore(db.DB)
			tt.setup(t, db.DB, store)

			got, err := ShouldNudge(ctx, store, fixtureRepoID, Account{Provider: "github", Login: "alice"})
			if err != nil {
				t.Fatalf("ShouldNudge: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ShouldNudge = %v, want %v", got, tt.want)
			}
		})
	}
}

func insertNudgeRepo(t *testing.T, db *sql.DB, repoID, source string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO repos (id, provider, owner, repo, watched, source) VALUES (?, 'github', 'owner', 'repo', 1, ?)`,
		repoID,
		source,
	)
	if err != nil {
		t.Fatalf("insert nudge repo: %v", err)
	}
}

func insertReviewedPRs(t *testing.T, db *sql.DB, repoID, accountProvider, accountLogin, login string, count int) {
	t.Helper()
	for index := 0; index < count; index++ {
		prID := repoID + "#" + string(rune('a'+index))
		_, err := db.Exec(
			`INSERT INTO prs (
				id, provider, owner, repo, number, title, author, state, created_at, updated_at,
				base_branch, head_branch, base_sha, head_sha, repo_id
			) VALUES (?, 'github', 'owner', 'repo', ?, 'PR', 'octocat', 'open', ?, ?, 'main', 'feature', 'base', 'head', ?)`,
			prID,
			index+1,
			"2026-07-15T12:00:00Z",
			"2026-07-15T12:00:00Z",
			repoID,
		)
		if err != nil {
			t.Fatalf("insert PR %d: %v", index, err)
		}
		_, err = db.Exec(
			`INSERT INTO last_reviewed_sha (pr_id, account_provider, account_login, login, sha) VALUES (?, ?, ?, ?, ?)`,
			prID,
			accountProvider,
			accountLogin,
			login,
			"sha",
		)
		if err != nil {
			t.Fatalf("insert last_reviewed_sha %d: %v", index, err)
		}
	}
}

func setNudgeState(t *testing.T, store *Store, state ExpertiseRepoState) {
	t.Helper()
	if err := store.SetRepoState(context.Background(), state); err != nil {
		t.Fatalf("SetRepoState: %v", err)
	}
}
