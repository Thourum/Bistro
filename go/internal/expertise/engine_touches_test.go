package expertise

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestEngineTouchesYouForPRsUsesPRDiffsAndTopThreeExpertRule(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db.DB)
	ctx := context.Background()
	repoID := "github:owner/repo"
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

	insertTouchPR(t, db.DB, "pr-qualifies", repoID, "qualifies.go", 1)
	insertTouchPR(t, db.DB, "pr-below-threshold", repoID, "below.go", 2)
	insertTouchPR(t, db.DB, "pr-other-user", repoID, "other.go", 3)

	if err := store.UpsertFileRows(ctx, []FileExpertiseRow{
		{RepoID: repoID, FilePath: "qualifies.go", AuthorEmail: "first@example.com", SurvivingLines: 80, TotalLines: 100, Score: 90, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "qualifies.go", AuthorEmail: "alice@example.com", SurvivingLines: DefaultMinSurvivingLines, TotalLines: 100, Score: 70, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "qualifies.go", AuthorEmail: "third@example.com", SurvivingLines: 80, TotalLines: 100, Score: 60, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "qualifies.go", AuthorEmail: "fourth@example.com", SurvivingLines: 80, TotalLines: 100, Score: 50, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "below.go", AuthorEmail: "alice@example.com", SurvivingLines: DefaultMinSurvivingLines - 1, TotalLines: 100, Score: ExpertScoreFloor - 1, ComputedAt: now, ComputedAtSHA: "sha"},
		{RepoID: repoID, FilePath: "other.go", AuthorEmail: "bob@example.com", SurvivingLines: 200, TotalLines: 250, Score: 95, ComputedAt: now, ComputedAtSHA: "sha"},
	}); err != nil {
		t.Fatalf("UpsertFileRows: %v", err)
	}

	indexer := NewIndexer(newSilentExpertiseLogger(), store, nil, func(context.Context) Identity {
		return Identity{Login: "alice", AliasEmails: []string{"alice@example.com"}}
	})
	engine := NewEngine(newSilentExpertiseLogger(), store, nil, indexer, nil)

	got, err := engine.TouchesYouForPRs(ctx, []string{"pr-qualifies", "pr-below-threshold", "pr-other-user"})
	if err != nil {
		t.Fatalf("TouchesYouForPRs: %v", err)
	}

	cases := map[string]bool{
		"pr-qualifies":       true,
		"pr-below-threshold": false,
		"pr-other-user":      false,
	}
	for prID, want := range cases {
		if got[prID] != want {
			t.Fatalf("TouchesYouForPRs[%s] = %v, want %v (full result %#v)", prID, got[prID], want, got)
		}
	}
}

func insertTouchPR(t *testing.T, db expertiseTestExecer, prID, repoID, filePath string, number int) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO prs (id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha, repo_id)
		VALUES (?, 'github', 'owner', 'repo', ?, 'title', 'author', 'open', ?, ?, 'main', 'feature', 'base', 'head', ?)`, prID, number, time.Now().UTC(), time.Now().UTC(), repoID); err != nil {
		t.Fatalf("insert pr %s: %v", prID, err)
	}
	if _, err := db.Exec(`INSERT INTO pr_diffs (pr_id, file_path, patch, additions, deletions) VALUES (?, ?, '', 0, 0)`, prID, filePath); err != nil {
		t.Fatalf("insert pr_diff %s %s: %v", prID, filePath, err)
	}
}

type expertiseTestExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}
