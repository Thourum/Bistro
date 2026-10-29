package services

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/store/storetest"
)

func TestRequestPRListIncludesInboxSignals(t *testing.T) {
	db := storetest.Open(t).DB
	viewer := &mockCurrentUserReader{login: "alice", found: true}
	updatedAt := time.Now().UTC().Add(-time.Minute)

	insertPRListTestPR(t, db, "commented", 1, "head-commented", true, updatedAt)
	insertPRListTestPR(t, db, "other-comment", 2, "head-other", false, updatedAt)
	insertPRListTestPR(t, db, "unread", 3, "head-unread", false, time.Now().UTC())
	insertPRListTestPR(t, db, "read", 4, "head-read", false, updatedAt)
	insertPRListTestPR(t, db, "no-read-state", 5, "head-no-read-state", false, updatedAt)
	insertPRListTestComment(t, db, "commented", "alice")
	insertPRListTestComment(t, db, "other-comment", "bob")

	for _, review := range []struct {
		prID string
		sha  string
	}{
		{prID: "commented", sha: "stale-sha"},
		{prID: "other-comment", sha: "head-other"},
	} {
		if err := store.RecordLastReviewed(context.Background(), db, store.ReviewEvent{
			PRID:       review.prID,
			Login:      "alice",
			HeadSHA:    review.sha,
			ReviewedAt: updatedAt,
			Source:     store.ReviewSourceBistroAction,
		}); err != nil {
			t.Fatalf("upsert last reviewed SHA for %s: %v", review.prID, err)
		}
	}

	if _, err := db.Exec(`INSERT INTO pr_read_state (pr_id, login, last_read_at) VALUES ('read', 'alice', ?)`, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	s := &PR{DB: db, CurrentUser: viewer}
	response, err := s.List(context.Background(), PRListRequest{Filter: "all", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if response.ViewerLogin != "alice" {
		t.Fatalf("viewer = %q", response.ViewerLogin)
	}
	prs := response.PRs

	byID := make(map[string]PRRow, len(prs))
	for _, pr := range prs {
		byID[pr.ID] = pr
	}

	assertPRListSignals(t, byID["commented"], true, true, true, true)
	assertPRListSignals(t, byID["other-comment"], false, false, true, false)
	assertPRListSignals(t, byID["unread"], false, false, true, false)
	assertPRListSignals(t, byID["read"], false, false, false, false)
	assertPRListSignals(t, byID["no-read-state"], false, false, true, false)
}

func TestRequestPRListUsesAnonymousDefaultsWithoutCurrentUser(t *testing.T) {
	db := storetest.Open(t).DB
	updatedAt := time.Now().UTC()
	insertPRListTestPR(t, db, "mentioned", 1, "head-mentioned", true, updatedAt)
	insertPRListTestComment(t, db, "mentioned", "alice")

	s := &PR{DB: db, CurrentUser: &mockCurrentUserReader{found: false}}
	response, err := s.List(context.Background(), PRListRequest{Filter: "all", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if response.ViewerLogin != "" {
		t.Fatalf("viewer = %q", response.ViewerLogin)
	}
	prs := response.PRs
	if len(prs) != 1 {
		t.Fatalf("PR count = %d, want 1", len(prs))
	}
	assertPRListSignals(t, prs[0], true, false, false, false)
}

func TestRequestPRListReturnsErrorWhenRowScanFails(t *testing.T) {
	db := storetest.Open(t).DB
	updatedAt := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO prs (
		id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha
	) VALUES ('bad-row', 'github', 'owner', 'repo', 'not-a-number', 'Test PR', 'author', 'open', ?, ?, 'main', 'feature', 'base', 'head')`, updatedAt, updatedAt); err != nil {
		t.Fatalf("insert malformed PR: %v", err)
	}

	_, err := (&PR{DB: db}).List(context.Background(), PRListRequest{Filter: "all", Limit: 10})
	assertServiceCode(t, err, CodeInternal)

}

func insertPRListTestPR(t *testing.T, db *sql.DB, prID string, number int, headSHA string, mentioned bool, updatedAt time.Time) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO prs (
		id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha, mentioned
	) VALUES (?, 'github', 'owner', 'repo', ?, 'Test PR', 'author', 'open', ?, ?, 'main', 'feature', 'base', ?, ?)`,
		prID, number, updatedAt, updatedAt, headSHA, mentioned); err != nil {
		t.Fatalf("insert PR %s: %v", prID, err)
	}
}

func insertPRListTestComment(t *testing.T, db *sql.DB, prID, author string) {
	t.Helper()
	createdAt := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO comments (id, pr_id, author, body, created_at, updated_at)
		VALUES (?, ?, ?, 'comment', ?, ?)`, prID+"-comment", prID, author, createdAt, createdAt); err != nil {
		t.Fatalf("insert comment for %s: %v", prID, err)
	}
}

func assertPRListSignals(t *testing.T, pr PRRow, mentioned, viewerCommented, unread, reviewedHeadStale bool) {
	t.Helper()
	if pr.Mentioned != mentioned || pr.ViewerCommented != viewerCommented || pr.Unread != unread || pr.ReviewedHeadStale != reviewedHeadStale {
		t.Errorf("signals for %s = mentioned:%t viewer_commented:%t unread:%t reviewed_head_stale:%t, want mentioned:%t viewer_commented:%t unread:%t reviewed_head_stale:%t",
			pr.ID, pr.Mentioned, pr.ViewerCommented, pr.Unread, pr.ReviewedHeadStale,
			mentioned, viewerCommented, unread, reviewedHeadStale)
	}
}

type mockCurrentUserReader struct {
	login string
	found bool
}

func (m *mockCurrentUserReader) Get(context.Context, string) (string, bool, error) {
	return m.login, m.found, nil
}

func TestPRListDefaultsAndLimitCap(t *testing.T) {
	db := storetest.Open(t).DB
	updatedAt := time.Now().UTC()
	for i := range maxPRListLimit + 1 {
		insertPRListTestPR(t, db, fmt.Sprintf("pr-%d", i), i, "head", false, updatedAt.Add(-time.Duration(i)*time.Minute))
	}
	if _, err := db.Exec(`UPDATE prs SET state = 'closed' WHERE id = 'pr-0'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE prs SET draft = 1 WHERE id = 'pr-1'`); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		request PRListRequest
		want    int
	}{
		{"default", PRListRequest{Offset: -1}, defaultPRListLimit},
		{"cap", PRListRequest{Filter: "all", Limit: maxPRListLimit + 1}, maxPRListLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := (&PR{DB: db}).List(context.Background(), test.request)
			if err != nil {
				t.Fatal(err)
			}
			if len(response.PRs) != test.want || response.Count != test.want || !response.HasMore {
				t.Fatalf("unexpected count: %+v", response)
			}
			if test.name == "default" {
				for _, pr := range response.PRs {
					if pr.State != "open" || pr.Draft != 0 {
						t.Fatalf("default filter returned %+v", pr)
					}
				}
			}
		})
	}
}
