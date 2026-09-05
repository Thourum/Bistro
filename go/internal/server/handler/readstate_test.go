package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type spyBroadcaster struct {
	messages []interface{}
}

func (s *spyBroadcaster) Broadcast(message interface{}) {
	s.messages = append(s.messages, message)
}

func newReadStateTestDB(t *testing.T) *store.DB {
	t.Helper()

	db, err := store.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := store.NewMigrationRunner(nil, db.DB).Run(context.Background()); err != nil {
		_ = db.Close()
		t.Fatalf("migrate test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func markPRsReadRequest(t *testing.T, prIDs []string) ws.InboundMessage {
	t.Helper()

	data, err := json.Marshal(map[string][]string{"pr_ids": prIDs})
	if err != nil {
		t.Fatalf("marshal MARK_PRS_READ request: %v", err)
	}
	return ws.InboundMessage{ID: "read-state-request", Type: "MARK_PRS_READ", Data: data}
}

func readStateResponseData(t *testing.T, response interface{}) map[string]interface{} {
	t.Helper()

	envelope, ok := response.(map[string]interface{})
	if !ok {
		t.Fatalf("response type = %T, want map[string]interface{}", response)
	}
	if envelope["type"] != "PRS_READ_ACK" {
		t.Errorf("response type = %v, want PRS_READ_ACK", envelope["type"])
	}
	return extractData(t, response)
}

func broadcastPayload(t *testing.T, message interface{}) map[string]interface{} {
	t.Helper()

	encoded, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal broadcast: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal broadcast: %v", err)
	}
	return payload
}

func TestMarkPRsReadHandlerMarksPRsAndBroadcasts(t *testing.T) {
	db := newReadStateTestDB(t)
	startedAt := time.Now().UTC().Add(-time.Second)
	broadcaster := &spyBroadcaster{}
	handler := NewMarkPRsReadHandler(db, &mockCurrentUserReader{login: "alice", found: true}, broadcaster, testRepoLogger())

	response, err := handler(context.Background(), markPRsReadRequest(t, []string{"p1", "p2"}))
	if err != nil {
		t.Fatalf("MARK_PRS_READ handler: %v", err)
	}
	data := readStateResponseData(t, response)
	if data["ok"] != true || data["marked"] != 2 {
		t.Errorf("response data = %#v, want ok=true and marked=2", data)
	}

	var rowCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pr_read_state WHERE login = 'alice' AND pr_id IN ('p1', 'p2')`).Scan(&rowCount); err != nil {
		t.Fatalf("count read-state rows: %v", err)
	}
	if rowCount != 2 {
		t.Errorf("read-state row count = %d, want 2", rowCount)
	}

	var lastReadAt time.Time
	if err := db.QueryRow(`SELECT last_read_at FROM pr_read_state WHERE pr_id = 'p1' AND login = 'alice'`).Scan(&lastReadAt); err != nil {
		t.Fatalf("query last_read_at: %v", err)
	}
	if lastReadAt.Before(startedAt) {
		t.Errorf("last_read_at = %s, want a timestamp after %s", lastReadAt, startedAt)
	}

	if len(broadcaster.messages) != 1 {
		t.Fatalf("broadcast count = %d, want 1", len(broadcaster.messages))
	}
	payload := broadcastPayload(t, broadcaster.messages[0])
	if payload["type"] != "PRS_READ_CHANGED" {
		t.Errorf("broadcast type = %v, want PRS_READ_CHANGED", payload["type"])
	}
	broadcastData, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("broadcast data = %T, want map[string]interface{}", payload["data"])
	}
	prIDs, ok := broadcastData["pr_ids"].([]interface{})
	if !ok || len(prIDs) != 2 || prIDs[0] != "p1" || prIDs[1] != "p2" {
		t.Errorf("broadcast pr_ids = %#v, want [p1 p2]", broadcastData["pr_ids"])
	}
}

func TestMarkPRsReadHandlerRejectsEmptyOrOversizedRequests(t *testing.T) {
	db := newReadStateTestDB(t)
	broadcaster := &spyBroadcaster{}
	handler := NewMarkPRsReadHandler(db, &mockCurrentUserReader{login: "alice", found: true}, broadcaster, testRepoLogger())

	for _, prIDs := range [][]string{nil, make([]string, 501)} {
		if _, err := handler(context.Background(), markPRsReadRequest(t, prIDs)); err == nil {
			t.Errorf("MARK_PRS_READ(%d PRs) error = nil, want validation error", len(prIDs))
		}
	}
	if len(broadcaster.messages) != 0 {
		t.Errorf("broadcast count = %d, want 0 after rejected requests", len(broadcaster.messages))
	}
}

func TestMarkPRsReadHandlerDeduplicatesPRIDs(t *testing.T) {
	db := newReadStateTestDB(t)
	broadcaster := &spyBroadcaster{}
	handler := NewMarkPRsReadHandler(db, &mockCurrentUserReader{login: "alice", found: true}, broadcaster, testRepoLogger())

	response, err := handler(context.Background(), markPRsReadRequest(t, []string{"p1", "p1"}))
	if err != nil {
		t.Fatalf("MARK_PRS_READ handler: %v", err)
	}
	data := readStateResponseData(t, response)
	if data["marked"] != 1 {
		t.Errorf("marked = %v, want 1 after deduplication", data["marked"])
	}

	var rowCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pr_read_state WHERE login = 'alice' AND pr_id = 'p1'`).Scan(&rowCount); err != nil {
		t.Fatalf("count read-state rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("read-state row count = %d, want 1", rowCount)
	}

	payload := broadcastPayload(t, broadcaster.messages[0])
	broadcastData := payload["data"].(map[string]interface{})
	prIDs := broadcastData["pr_ids"].([]interface{})
	if len(prIDs) != 1 || prIDs[0] != "p1" {
		t.Errorf("broadcast pr_ids = %#v, want [p1]", prIDs)
	}
}

func TestMarkPRsReadHandlerSkipsWithoutCurrentUser(t *testing.T) {
	db := newReadStateTestDB(t)
	broadcaster := &spyBroadcaster{}
	handler := NewMarkPRsReadHandler(db, &mockCurrentUserReader{found: false}, broadcaster, testRepoLogger())

	response, err := handler(context.Background(), markPRsReadRequest(t, []string{"p1"}))
	if err != nil {
		t.Fatalf("MARK_PRS_READ handler: %v", err)
	}
	data := readStateResponseData(t, response)
	if data["ok"] != false || data["marked"] != 0 {
		t.Errorf("response data = %#v, want ok=false and marked=0", data)
	}

	var rowCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pr_read_state`).Scan(&rowCount); err != nil {
		t.Fatalf("count read-state rows: %v", err)
	}
	if rowCount != 0 {
		t.Errorf("read-state row count = %d, want 0", rowCount)
	}
	if len(broadcaster.messages) != 0 {
		t.Errorf("broadcast count = %d, want 0", len(broadcaster.messages))
	}
}

func TestMarkPRsReadHandlerTimestampIsComparableWithPRUpdatedAt(t *testing.T) {
	db := newReadStateTestDB(t)
	updatedAt := time.Now().UTC().Add(-time.Minute)
	if _, err := db.Exec(`INSERT INTO prs (
		id, provider, owner, repo, number, title, author, state,
		created_at, updated_at, base_branch, head_branch, base_sha, head_sha
	) VALUES (?, 'github', 'owner', 'repo', 1, 'Test PR', 'author', 'open', ?, ?, 'main', 'feature', 'base', 'head')`,
		"p1", updatedAt, updatedAt); err != nil {
		t.Fatalf("insert PR: %v", err)
	}

	handler := NewMarkPRsReadHandler(db, &mockCurrentUserReader{login: "alice", found: true}, &spyBroadcaster{}, testRepoLogger())
	if _, err := handler(context.Background(), markPRsReadRequest(t, []string{"p1"})); err != nil {
		t.Fatalf("MARK_PRS_READ handler: %v", err)
	}

	var isUnread int
	if err := db.QueryRow(`SELECT COALESCE((
		SELECT unixepoch(r.last_read_at)
		FROM pr_read_state r
		WHERE r.pr_id = prs.id AND r.login = 'alice'
	), 0) < unixepoch(prs.updated_at) FROM prs WHERE id = 'p1'`).Scan(&isUnread); err != nil {
		t.Fatalf("evaluate unread expression: %v", err)
	}
	if isUnread != 0 {
		t.Errorf("unread expression = %d, want 0 after marking read", isUnread)
	}
}
