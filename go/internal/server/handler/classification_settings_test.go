package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/itterno/bistro/internal/ws"
)

// fakeKeyStore records keychain calls in memory; asserts key material never
// leaks through responses.
type fakeKeyStore struct {
	stored  map[string]string
	deleted []string
	failSet bool
}

func newFakeKeyStore() *fakeKeyStore {
	return &fakeKeyStore{stored: map[string]string{}}
}

func (f *fakeKeyStore) Store(provider, token string) error {
	if f.failSet {
		return fmt.Errorf("keychain unavailable")
	}
	f.stored[provider] = token
	return nil
}

func (f *fakeKeyStore) Delete(provider string) error {
	f.deleted = append(f.deleted, provider)
	delete(f.stored, provider)
	return nil
}

func callHandler(t *testing.T, h ws.MessageHandler, msgType string, payload interface{}) (interface{}, error) {
	t.Helper()
	data, _ := json.Marshal(payload)
	return h(context.Background(), ws.InboundMessage{ID: "r1", Type: msgType, Data: data})
}

func TestSetClassificationSettings(t *testing.T) {
	state := NewClassificationState()
	h := NewSetClassificationSettingsHandler(state, testRepoLogger())

	resp, err := callHandler(t, h, "SET_CLASSIFICATION_SETTINGS", map[string]string{
		"mode": "rules+llm", "dismissal_scope": "local+provider",
		"provider": "openai", "model": "gpt-5", "endpoint": "",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	env := resp.(map[string]interface{})
	if env["type"] != "CLASSIFICATION_SETTINGS_UPDATED" {
		t.Errorf("ack type = %v", env["type"])
	}
	if state.Mode() != "rules+llm" || state.DismissalScope() != "local+provider" {
		t.Errorf("state not updated: %+v", state.Snapshot())
	}
}

func TestSetClassificationSettingsInvalidMode(t *testing.T) {
	state := NewClassificationState()
	h := NewSetClassificationSettingsHandler(state, testRepoLogger())

	_, err := callHandler(t, h, "SET_CLASSIFICATION_SETTINGS", map[string]string{
		"mode": "catastrophic", "dismissal_scope": "local",
	})

	if err == nil {
		t.Fatalf("invalid mode must error")
	}
	if state.Mode() != "rules" {
		t.Errorf("state changed on invalid input: %q", state.Mode())
	}
}

func TestSetClassificationSettingsServerStubAccepted(t *testing.T) {
	state := NewClassificationState()
	h := NewSetClassificationSettingsHandler(state, testRepoLogger())

	_, err := callHandler(t, h, "SET_CLASSIFICATION_SETTINGS", map[string]string{
		"mode": "server", "dismissal_scope": "local", "provider": "anthropic",
	})

	if err != nil {
		t.Fatalf("server stub mode must be accepted: %v", err)
	}
	if state.Mode() != "server" {
		t.Errorf("mode = %q, want server", state.Mode())
	}
}

func TestClassificationStateConcurrentAccess(t *testing.T) {
	state := NewClassificationState()
	h := NewSetClassificationSettingsHandler(state, testRepoLogger())

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = callHandler(t, h, "SET_CLASSIFICATION_SETTINGS", map[string]string{
				"mode": "llm-all", "dismissal_scope": "local", "provider": "anthropic",
			})
		}()
		go func() {
			defer wg.Done()
			_ = state.Mode()
			_ = state.DismissalScope()
		}()
	}
	wg.Wait()
}

func TestSetByokKeyStoresInKeychainOnly(t *testing.T) {
	store := newFakeKeyStore()
	changed := false
	h := NewSetByokKeyHandler(store, func() { changed = true }, testRepoLogger())

	resp, err := callHandler(t, h, "SET_BYOK_KEY", map[string]string{
		"provider": "anthropic", "key": "sk-secret-value",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if store.stored["byok.anthropic"] != "sk-secret-value" {
		t.Errorf("key not stored under byok.anthropic: %v", store.stored)
	}
	if !changed {
		t.Errorf("onKeyChanged not invoked")
	}
	// The key must NEVER appear anywhere in the response.
	raw, _ := json.Marshal(resp)
	if strings.Contains(string(raw), "sk-secret-value") {
		t.Errorf("API key leaked into response: %s", raw)
	}
	env := resp.(map[string]interface{})
	if env["type"] != "BYOK_KEY_UPDATED" {
		t.Errorf("ack type = %v", env["type"])
	}
}

func TestSetByokKeyStoreFailureNoKeyInError(t *testing.T) {
	store := newFakeKeyStore()
	store.failSet = true
	h := NewSetByokKeyHandler(store, nil, testRepoLogger())

	_, err := callHandler(t, h, "SET_BYOK_KEY", map[string]string{
		"provider": "openai", "key": "sk-super-secret",
	})

	if err == nil {
		t.Fatalf("store failure must error")
	}
	if strings.Contains(err.Error(), "sk-super-secret") {
		t.Errorf("API key leaked into error: %v", err)
	}
}

func TestDeleteByokKey(t *testing.T) {
	store := newFakeKeyStore()
	store.stored["byok.ollama"] = "k"
	changed := false
	h := NewDeleteByokKeyHandler(store, func() { changed = true }, testRepoLogger())

	resp, err := callHandler(t, h, "DELETE_BYOK_KEY", map[string]string{"provider": "ollama"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if len(store.stored) != 0 || len(store.deleted) != 1 || store.deleted[0] != "byok.ollama" {
		t.Errorf("key not deleted: stored=%v deleted=%v", store.stored, store.deleted)
	}
	if !changed {
		t.Errorf("onKeyChanged not invoked")
	}
	env := resp.(map[string]interface{})
	data := env["data"].(map[string]interface{})
	if data["present"] != false {
		t.Errorf("present = %v, want false", data["present"])
	}
}
