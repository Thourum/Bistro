package handler

import (
	"context"
	"database/sql"
	"testing"
	"time"

	storedb "github.com/itterno/bistro/internal/store/db"
)

func TestRequestPRListIncludesInboxSignals(t *testing.T) {
	db := newHandlerTestDB(t)
	viewer := &mockCurrentUserReader{login: "alice", found: true}
	updatedAt := time.Now().UTC().Add(-time.Minute)

	insertPRListTestPR(t, db, "commented", 1, "head-commented", true, updatedAt)
	insertPRListTestPR(t, db, "other-comment", 2, "head-other", false, updatedAt)
	insertPRListTestPR(t, db, "unread", 3, "head-unread", false, time.Now().UTC())
	insertPRListTestPR(t, db, "read", 4, "head-read", false, updatedAt)
	insertPRListTestPR(t, db, "no-read-state", 5, "head-no-read-state", false, updatedAt)
	insertPRListTestComment(t, db, "commented", "alice")
	insertPRListTestComment(t, db, "other-comment", "bob")

	queries := storedb.New(db)
	for _, review := range []struct {
		prID string
		sha  string
	}{
		{prID: "commented", sha: "stale-sha"},
		{prID: "other-comment", sha: "head-other"},
	} {
		if err := queries.UpsertLastReviewedSHA(context.Background(), storedb.UpsertLastReviewedSHAParams{
			PrID:            review.prID,
			AccountProvider: "github",
			AccountLogin:    "alice",
			Login:           "alice",
			Sha:             review.sha,
		}); err != nil {
			t.Fatalf("upsert last reviewed SHA for %s: %v", review.prID, err)
		}
	}

	markRead := NewMarkPRsReadHandler(db, viewer, &spyBroadcaster{}, testRepoLogger())
	if _, err := markRead(context.Background(), markPRsReadRequest(t, []string{"read"})); err != nil {
		t.Fatalf("mark read: %v", err)
	}

	handler := NewRequestPRListHandlerWithExpertise(db, nil, viewer, testRepoLogger())
	response, err := callHandler(t, handler, "REQUEST_PR_LIST", map[string]any{"filter": "all", "limit": 10})
	if err != nil {
		t.Fatalf("REQUEST_PR_LIST: %v", err)
	}
	data := extractData(t, response)
	if got := data["viewer_login"]; got != "alice" {
		t.Fatalf("viewer_login = %v, want alice", got)
	}

	prs, ok := data["prs"].([]prRow)
	if !ok {
		t.Fatalf("prs = %T, want []prRow", data["prs"])
	}
	byID := make(map[string]prRow, len(prs))
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
	db := newHandlerTestDB(t)
	updatedAt := time.Now().UTC()
	insertPRListTestPR(t, db, "mentioned", 1, "head-mentioned", true, updatedAt)
	insertPRListTestComment(t, db, "mentioned", "alice")

	handler := NewRequestPRListHandlerWithExpertise(db, nil, &mockCurrentUserReader{found: false}, testRepoLogger())
	response, err := callHandler(t, handler, "REQUEST_PR_LIST", map[string]any{"filter": "all", "limit": 10})
	if err != nil {
		t.Fatalf("REQUEST_PR_LIST: %v", err)
	}
	data := extractData(t, response)
	if got := data["viewer_login"]; got != "" {
		t.Fatalf("viewer_login = %v, want empty string", got)
	}

	prs := data["prs"].([]prRow)
	if len(prs) != 1 {
		t.Fatalf("PR count = %d, want 1", len(prs))
	}
	assertPRListSignals(t, prs[0], true, false, false, false)
}

func TestRequestPRListReturnsErrorWhenRowScanFails(t *testing.T) {
	db := newHandlerTestDB(t)
	updatedAt := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO prs (
		id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha
	) VALUES ('bad-row', 'github', 'owner', 'repo', 'not-a-number', 'Test PR', 'author', 'open', ?, ?, 'main', 'feature', 'base', 'head')`, updatedAt, updatedAt); err != nil {
		t.Fatalf("insert malformed PR: %v", err)
	}

	handler := NewRequestPRListHandler(db, &mockCurrentUserReader{found: false}, testRepoLogger())
	response, err := callHandler(t, handler, "REQUEST_PR_LIST", map[string]any{"filter": "all", "limit": 10})
	if err != nil {
		t.Fatalf("REQUEST_PR_LIST: %v", err)
	}
	data := extractData(t, response)
	if got := data["count"]; got != 0 {
		t.Errorf("count = %v, want 0", got)
	}
	if got, ok := data["prs"].([]interface{}); !ok || len(got) != 0 {
		t.Errorf("prs = %#v, want empty list", data["prs"])
	}
	if got, ok := data["error"].(string); !ok || got == "" {
		t.Errorf("error = %#v, want non-empty string", data["error"])
	}
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

func assertPRListSignals(t *testing.T, pr prRow, mentioned, viewerCommented, unread, reviewedHeadStale bool) {
	t.Helper()
	if pr.Mentioned != mentioned || pr.ViewerCommented != viewerCommented || pr.Unread != unread || pr.ReviewedHeadStale != reviewedHeadStale {
		t.Errorf("signals for %s = mentioned:%t viewer_commented:%t unread:%t reviewed_head_stale:%t, want mentioned:%t viewer_commented:%t unread:%t reviewed_head_stale:%t",
			pr.ID, pr.Mentioned, pr.ViewerCommented, pr.Unread, pr.ReviewedHeadStale,
			mentioned, viewerCommented, unread, reviewedHeadStale)
	}
}
