package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/samuelasvanyi/bistro/internal/ws"
)

func setupCommentsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE comments (
		id          TEXT     PRIMARY KEY,
		pr_id       TEXT     NOT NULL,
		author      TEXT     NOT NULL,
		body        TEXT     NOT NULL,
		created_at  DATETIME NOT NULL,
		updated_at  DATETIME NOT NULL,
		line_number INTEGER,
		file_path   TEXT
	)`)
	if err != nil {
		t.Fatalf("create comments table: %v", err)
	}
	return db
}

func insertTestComment(t *testing.T, db *sql.DB, id, prID, author, body string, line *int, file *string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO comments (id, pr_id, author, body, created_at, updated_at, line_number, file_path)
		 VALUES (?, ?, ?, ?, '2026-06-01T10:00:00Z', '2026-06-01T10:00:00Z', ?, ?)`,
		id, prID, author, body, line, file,
	)
	if err != nil {
		t.Fatalf("insert comment: %v", err)
	}
}

func commentsResponse(t *testing.T, resp interface{}) (string, map[string]interface{}) {
	t.Helper()
	env, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("response is not a map, got %T", resp)
	}
	msgType, _ := env["type"].(string)
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data is not a map, got %T", env["data"])
	}
	return msgType, data
}

func TestRequestCommentsHandler_ReturnsComments(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()

	line := 42
	file := "main.go"
	insertTestComment(t, db, "c1", "github:acme/api:1", "alice", "looks good", &line, &file)
	insertTestComment(t, db, "c2", "github:acme/api:1", "bob", "needs work", nil, nil)
	insertTestComment(t, db, "c3", "github:acme/api:2", "carol", "other PR", nil, nil)

	h := NewRequestCommentsHandler(db, testRepoLogger())

	payload, _ := json.Marshal(map[string]string{"pr_id": "github:acme/api:1"})
	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-1",
		Type: "REQUEST_COMMENTS",
		Data: payload,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	msgType, data := commentsResponse(t, resp)
	if msgType != "COMMENTS_READY" {
		t.Errorf("type = %q, want COMMENTS_READY", msgType)
	}
	if data["pr_id"] != "github:acme/api:1" {
		t.Errorf("pr_id = %v, want github:acme/api:1", data["pr_id"])
	}

	comments, ok := data["comments"].([]commentRow)
	if !ok {
		t.Fatalf("comments is %T, want []commentRow", data["comments"])
	}
	if len(comments) != 2 {
		t.Fatalf("got %d comments, want 2", len(comments))
	}
	first := comments[0]
	if first.ID != "c1" || first.Author != "alice" {
		t.Errorf("first comment = %+v, want c1/alice", first)
	}
	if first.Line == nil || *first.Line != 42 {
		t.Errorf("first comment line = %v, want 42", first.Line)
	}
	if first.File == nil || *first.File != "main.go" {
		t.Errorf("first comment file = %v, want main.go", first.File)
	}
	if comments[1].Line != nil {
		t.Errorf("second comment line = %v, want nil", comments[1].Line)
	}
}

func TestRequestCommentsHandler_EmptyResult(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()

	h := NewRequestCommentsHandler(db, testRepoLogger())

	payload, _ := json.Marshal(map[string]string{"pr_id": "github:acme/api:999"})
	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-2",
		Type: "REQUEST_COMMENTS",
		Data: payload,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	msgType, data := commentsResponse(t, resp)
	if msgType != "COMMENTS_READY" {
		t.Errorf("type = %q, want COMMENTS_READY", msgType)
	}
	comments, ok := data["comments"].([]commentRow)
	if !ok {
		t.Fatalf("comments is %T, want []commentRow", data["comments"])
	}
	if len(comments) != 0 {
		t.Errorf("got %d comments, want 0 (must be empty slice, not nil)", len(comments))
	}
}

func TestRequestCommentsHandler_InvalidPayload(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()

	h := NewRequestCommentsHandler(db, testRepoLogger())

	_, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-3",
		Type: "REQUEST_COMMENTS",
		Data: json.RawMessage(`{not json`),
	})
	if err == nil {
		t.Error("expected error for invalid payload, got nil")
	}
}

func TestRequestCommentsHandler_MissingPRID(t *testing.T) {
	db := setupCommentsTestDB(t)
	defer db.Close()

	h := NewRequestCommentsHandler(db, testRepoLogger())

	payload, _ := json.Marshal(map[string]string{})
	_, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-4",
		Type: "REQUEST_COMMENTS",
		Data: payload,
	})
	if err == nil {
		t.Error("expected error for missing pr_id, got nil")
	}
}
