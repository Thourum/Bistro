package handler

import "testing"

func TestCommentsServeDismissedPerAccount(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()
	insertTestComment(t, db, "c1", "pr-1", "coderabbitai[bot]", "nit: x", nil, nil)
	insertTestComment(t, db, "c2", "pr-1", "alice", "fine", nil, nil)
	// samdev dismissed c1; a different account dismissed c2.
	if _, err := db.Exec(`INSERT INTO current_user (provider, login) VALUES ('github','samdev')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO comment_dismissals (comment_id, account_provider, account_login) VALUES
		('c1','github','samdev'), ('c2','github','someone-else')`); err != nil {
		t.Fatalf("seed dismissals: %v", err)
	}

	comments := requestComments(t, NewRequestCommentsHandler(db, testRepoLogger()), "pr-1")

	if len(comments) != 2 {
		t.Fatalf("got %d comments, want 2", len(comments))
	}
	byID := map[string]commentRow{}
	for _, c := range comments {
		byID[c.ID] = c
	}
	if !byID["c1"].Dismissed {
		t.Errorf("c1 dismissed by current account must serve dismissed=true")
	}
	if byID["c2"].Dismissed {
		t.Errorf("c2 dismissed by ANOTHER account must serve dismissed=false")
	}
}
