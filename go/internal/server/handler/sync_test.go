package handler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/itterno/bistro/internal/ws"
)

func forceSyncResponse(t *testing.T, resp interface{}) (string, map[string]interface{}) {
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

func TestForceSyncHandler_TriggersAllRepos(t *testing.T) {
	trigger := make(chan string, 1)
	h := NewForceSyncHandler(trigger, testRepoLogger())

	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-1",
		Type: "FORCE_SYNC",
		Data: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	msgType, data := forceSyncResponse(t, resp)
	if msgType != "SYNC_TRIGGERED" {
		t.Errorf("type = %q, want SYNC_TRIGGERED", msgType)
	}
	if data["queued"] != true {
		t.Errorf("queued = %v, want true", data["queued"])
	}

	select {
	case repoID := <-trigger:
		if repoID != "" {
			t.Errorf("trigger repo_id = %q, want empty (all repos)", repoID)
		}
	default:
		t.Error("expected a value on the sync trigger channel")
	}
}

func TestForceSyncHandler_TriggersSingleRepo(t *testing.T) {
	trigger := make(chan string, 1)
	h := NewForceSyncHandler(trigger, testRepoLogger())

	payload, _ := json.Marshal(map[string]string{"repo_id": "github:acme/api"})
	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-2",
		Type: "FORCE_SYNC",
		Data: payload,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	_, data := forceSyncResponse(t, resp)
	if data["queued"] != true {
		t.Errorf("queued = %v, want true", data["queued"])
	}

	select {
	case repoID := <-trigger:
		if repoID != "github:acme/api" {
			t.Errorf("trigger repo_id = %q, want github:acme/api", repoID)
		}
	default:
		t.Error("expected a value on the sync trigger channel")
	}
}

func TestForceSyncHandler_ChannelFull(t *testing.T) {
	trigger := make(chan string) // unbuffered, no receiver → send would block
	h := NewForceSyncHandler(trigger, testRepoLogger())

	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-3",
		Type: "FORCE_SYNC",
		Data: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	msgType, data := forceSyncResponse(t, resp)
	if msgType != "SYNC_TRIGGERED" {
		t.Errorf("type = %q, want SYNC_TRIGGERED", msgType)
	}
	if data["queued"] != false {
		t.Errorf("queued = %v, want false when channel is full", data["queued"])
	}
}

func TestForceSyncHandler_EmptyPayload(t *testing.T) {
	trigger := make(chan string, 1)
	h := NewForceSyncHandler(trigger, testRepoLogger())

	// Renderer may send no data at all.
	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "req-4",
		Type: "FORCE_SYNC",
		Data: nil,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	_, data := forceSyncResponse(t, resp)
	if data["queued"] != true {
		t.Errorf("queued = %v, want true", data["queued"])
	}
}
