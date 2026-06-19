package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/itterno/bistro/internal/store"
	"github.com/itterno/bistro/internal/ws"
)

type stubDiffReader struct {
	diffs map[string]string
	files map[string][]store.DiffFile
}

func (s *stubDiffReader) GetUnifiedDiff(prID string) (string, error) {
	if d, ok := s.diffs[prID]; ok {
		return d, nil
	}
	return "", fmt.Errorf("no diffs found for PR %s", prID)
}

func (s *stubDiffReader) GetDiffs(prID string) ([]store.DiffFile, error) {
	return s.files[prID], nil
}

func makeMsg(t *testing.T, id, prID string) ws.InboundMessage {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"pr_id": prID})
	return ws.InboundMessage{ID: id, Type: "REQUEST_DIFF", Data: data}
}

func TestRequestDiffHandler_Found(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{
		"pr-1": "diff --git a/main.go b/main.go\n+new line\n",
	}}
	handler := NewRequestDiffHandler(stub, nil)

	resp, err := handler(context.Background(), makeMsg(t, "req-1", "pr-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got %T", resp)
	}
	if m["type"] != "DIFF_READY" {
		t.Errorf("expected type DIFF_READY, got %v", m["type"])
	}
	if m["id"] != "req-1" {
		t.Errorf("expected id req-1, got %v", m["id"])
	}

	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got %T", m["data"])
	}
	if data["pr_id"] != "pr-1" {
		t.Errorf("expected pr_id pr-1, got %v", data["pr_id"])
	}
	if data["diff"] == "" {
		t.Error("expected non-empty diff")
	}
	if _, hasErr := data["error"]; hasErr {
		t.Errorf("expected no error field on success, got %v", data["error"])
	}
}

func TestRequestDiffHandler_IncludesFileStats(t *testing.T) {
	stub := &stubDiffReader{
		diffs: map[string]string{"pr-1": "diff --git a/a.go b/a.go\n+x\n"},
		files: map[string][]store.DiffFile{
			"pr-1": {
				{FilePath: "a.go", Additions: 10, Deletions: 2},
				{FilePath: "b.go", Additions: 5, Deletions: 1},
			},
		},
	}
	handler := NewRequestDiffHandler(stub, nil)

	resp, err := handler(context.Background(), makeMsg(t, "req-files", "pr-1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := resp.(map[string]interface{})["data"].(map[string]interface{})

	files, ok := data["files"].([]diffFileStat)
	if !ok {
		t.Fatalf("files is %T, want []diffFileStat", data["files"])
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2", len(files))
	}
	if files[0].Path != "a.go" || files[0].Additions != 10 || files[0].Deletions != 2 {
		t.Errorf("unexpected first file: %+v", files[0])
	}
	if data["additions"] != 15 {
		t.Errorf("additions = %v, want 15", data["additions"])
	}
	if data["deletions"] != 3 {
		t.Errorf("deletions = %v, want 3", data["deletions"])
	}
}

func TestRequestDiffHandler_NotFound(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{}}
	handler := NewRequestDiffHandler(stub, nil)

	resp, err := handler(context.Background(), makeMsg(t, "req-2", "pr-missing"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := resp.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map response, got %T", resp)
	}
	if m["type"] != "DIFF_READY" {
		t.Errorf("expected type DIFF_READY, got %v", m["type"])
	}

	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got %T", m["data"])
	}
	if data["error"] == "" {
		t.Error("expected non-empty error field for missing PR")
	}
	if data["diff"] != "" {
		t.Errorf("expected empty diff on error, got %v", data["diff"])
	}
}

func TestRequestDiffHandler_MissingPRID(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{}}
	handler := NewRequestDiffHandler(stub, nil)

	data, _ := json.Marshal(map[string]string{})
	msg := ws.InboundMessage{ID: "req-3", Type: "REQUEST_DIFF", Data: data}

	_, err := handler(context.Background(), msg)
	if err == nil {
		t.Error("expected error for missing pr_id, got nil")
	}
}

func TestRequestDiffHandler_InvalidPayload(t *testing.T) {
	stub := &stubDiffReader{diffs: map[string]string{}}
	handler := NewRequestDiffHandler(stub, nil)

	msg := ws.InboundMessage{ID: "req-4", Type: "REQUEST_DIFF", Data: []byte("not-json")}

	_, err := handler(context.Background(), msg)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}
