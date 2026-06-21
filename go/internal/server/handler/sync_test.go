package handler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

func TestSetSyncSettingsHandler_UpdatesSettings(t *testing.T) {
	ch := make(chan SyncSettingsMsg, 1)
	h := NewSetSyncSettingsHandler(ch, nil)

	payload, _ := json.Marshal(map[string]interface{}{
		"auto_sync":          true,
		"sync_interval_secs": 120.0,
	})
	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "s-1",
		Type: "SET_SYNC_SETTINGS",
		Data: payload,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	env := resp.(map[string]interface{})
	if env["type"] != "SYNC_SETTINGS_UPDATED" {
		t.Errorf("type = %v, want SYNC_SETTINGS_UPDATED", env["type"])
	}
	data := env["data"].(map[string]interface{})
	if data["applied"] != true {
		t.Errorf("applied = %v, want true", data["applied"])
	}

	select {
	case msg := <-ch:
		if !msg.AutoSync {
			t.Error("AutoSync should be true")
		}
		if msg.Interval != 120*time.Second {
			t.Errorf("Interval = %v, want 120s", msg.Interval)
		}
	default:
		t.Error("expected a SyncSettingsMsg on the channel")
	}
}

func TestSetSyncSettingsHandler_DisableAutoSync(t *testing.T) {
	ch := make(chan SyncSettingsMsg, 1)
	h := NewSetSyncSettingsHandler(ch, nil)

	payload, _ := json.Marshal(map[string]interface{}{
		"auto_sync":          false,
		"sync_interval_secs": 60.0,
	})
	_, err := h(context.Background(), ws.InboundMessage{
		ID:   "s-2",
		Type: "SET_SYNC_SETTINGS",
		Data: payload,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	msg := <-ch
	if msg.AutoSync {
		t.Error("AutoSync should be false")
	}
}

func TestSetSyncSettingsHandler_InvalidInterval(t *testing.T) {
	ch := make(chan SyncSettingsMsg, 1)
	h := NewSetSyncSettingsHandler(ch, nil)

	payload, _ := json.Marshal(map[string]interface{}{
		"auto_sync":          true,
		"sync_interval_secs": 0,
	})
	_, err := h(context.Background(), ws.InboundMessage{
		ID:   "s-3",
		Type: "SET_SYNC_SETTINGS",
		Data: payload,
	})
	if err == nil {
		t.Error("expected error for zero interval, got nil")
	}
}

func TestSetSyncSettingsHandler_ChannelFull_Coalesces(t *testing.T) {
	// Channel with capacity 1, pre-filled with a stale value.
	ch := make(chan SyncSettingsMsg, 1)
	ch <- SyncSettingsMsg{AutoSync: true, Interval: 999 * time.Second} // stale

	h := NewSetSyncSettingsHandler(ch, nil)

	payload, _ := json.Marshal(map[string]interface{}{
		"auto_sync":          false,
		"sync_interval_secs": 30.0,
	})
	resp, err := h(context.Background(), ws.InboundMessage{
		ID:   "s-4",
		Type: "SET_SYNC_SETTINGS",
		Data: payload,
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	env := resp.(map[string]interface{})
	data := env["data"].(map[string]interface{})
	// After coalescing the stale entry, the new value must be applied.
	if data["applied"] != true {
		t.Errorf("applied = %v, want true (coalescing should have freed the slot)", data["applied"])
	}

	// The channel must hold the latest value, not the stale one.
	select {
	case msg := <-ch:
		if msg.AutoSync != false {
			t.Errorf("AutoSync = %v, want false (latest value)", msg.AutoSync)
		}
		if msg.Interval != 30*time.Second {
			t.Errorf("Interval = %v, want 30s", msg.Interval)
		}
	default:
		t.Error("expected the latest SyncSettingsMsg on the channel")
	}
}
