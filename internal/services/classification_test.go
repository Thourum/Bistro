package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

type fakeKeyStore struct {
	stored    map[string]string
	storeErr  error
	deleteErr error
}

func (f *fakeKeyStore) Store(provider, key string) error {
	if f.storeErr != nil {
		return f.storeErr
	}
	f.stored[provider] = key
	return nil
}
func (f *fakeKeyStore) Delete(provider string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.stored[provider]; !ok {
		return keyring.ErrNotFound
	}
	delete(f.stored, provider)
	return nil
}

func TestClassificationSettingsValidation(t *testing.T) {
	state := NewClassificationState()
	s := &Classification{State: state}
	for _, mode := range []string{"rules", "rules+llm", "llm-all", "server"} {
		req := ClassificationSettings{Mode: mode, DismissalScope: "local+provider", Provider: "openai", Model: "gpt-5", Endpoint: "http://localhost"}
		got, err := s.SetSettings(context.Background(), req)
		if err != nil || got != req || state.Snapshot() != req || state.Mode() != mode || state.DismissalScope() != req.DismissalScope {
			t.Fatalf("settings not applied: %v", err)
		}
	}
	before := state.Snapshot()
	for _, req := range []ClassificationSettings{{Mode: "invalid", DismissalScope: "local"}, {Mode: "rules", DismissalScope: "invalid"}} {
		_, err := s.SetSettings(context.Background(), req)
		var se *ServiceError
		if !errors.As(err, &se) || se.Code != CodeInvalid || state.Snapshot() != before {
			t.Fatalf("invalid settings changed state: %v", err)
		}
	}
}

func TestClassificationStateConcurrentAccess(t *testing.T) {
	state := NewClassificationState()
	s := &Classification{State: state}
	var wg sync.WaitGroup
	const concurrentUpdates = 10
	for i := 0; i < concurrentUpdates; i++ {
		wg.Go(func() {
			req := ClassificationSettings{Mode: "llm-all", DismissalScope: "local", Provider: "anthropic"}
			if _, err := s.SetSettings(context.Background(), req); err != nil {
				t.Error(err)
			}
		})
		wg.Go(func() { _ = state.Mode(); _ = state.DismissalScope(); _ = state.Snapshot() })
	}
	wg.Wait()
	if state.Mode() != "llm-all" {
		t.Fatal("concurrent update lost")
	}
}

func TestByokSetDeletePresence(t *testing.T) {
	keys := &fakeKeyStore{stored: map[string]string{}}
	changes := 0
	s := &Classification{Keys: keys, OnKeyChanged: func() { changes++ }}
	const secret = "test-secret-value"
	got, err := s.SetByokKey(context.Background(), ByokKeyRequest{Provider: "anthropic", Key: secret})
	if err != nil || !got.Present || got.Provider != "anthropic" || keys.stored["byok.anthropic"] != secret {
		t.Fatalf("set status = %+v, error = %v", got, err)
	}
	raw, err := json.Marshal(got)
	if err != nil || strings.Contains(string(raw), secret) {
		t.Fatal("secret exposed in response")
	}
	for range 2 {
		got, err = s.DeleteByokKey(context.Background(), ByokProviderRequest{Provider: "anthropic"})
		if err != nil || got.Present || got.Provider != "anthropic" || len(keys.stored) != 0 {
			t.Fatalf("delete status = %+v, error = %v", got, err)
		}
	}
	if changes != 3 {
		t.Fatalf("key changes = %d", changes)
	}
}

func TestByokFailuresAreSanitized(t *testing.T) {
	const secret = "test-secret-value"
	for _, cause := range []error{fmt.Errorf("keychain failed: %s", secret), context.Canceled, context.DeadlineExceeded} {
		var logs bytes.Buffer
		keys := &fakeKeyStore{stored: map[string]string{"byok.openai": secret}, storeErr: cause, deleteErr: cause}
		changes := 0
		s := &Classification{Keys: keys, Log: slog.New(slog.NewTextHandler(&logs, nil)), OnKeyChanged: func() { changes++ }}
		_, storeErr := s.SetByokKey(context.Background(), ByokKeyRequest{Provider: "openai", Key: secret})
		_, deleteErr := s.DeleteByokKey(context.Background(), ByokProviderRequest{Provider: "openai"})
		want := CodeInternal
		if errors.Is(cause, context.Canceled) {
			want = CodeCancelled
		}
		if errors.Is(cause, context.DeadlineExceeded) {
			want = CodeTimeout
		}
		for _, err := range []error{storeErr, deleteErr} {
			var se *ServiceError
			if !errors.As(err, &se) || se.Code != want || strings.Contains(err.Error(), secret) {
				t.Fatalf("unsafe error: %v", err)
			}
		}
		if strings.Contains(logs.String(), secret) || changes != 0 || keys.stored["byok.openai"] != secret {
			t.Fatal("failed mutation leaked secret or reported success")
		}
	}
}

func TestByokRequiresFields(t *testing.T) {
	s := &Classification{}
	for _, req := range []ByokKeyRequest{{}, {Provider: "openai"}, {Key: "test-key"}} {
		_, err := s.SetByokKey(context.Background(), req)
		var se *ServiceError
		if !errors.As(err, &se) || se.Code != CodeInvalid {
			t.Fatalf("error = %v", err)
		}
	}
	_, err := s.DeleteByokKey(context.Background(), ByokProviderRequest{})
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != CodeInvalid {
		t.Fatalf("error = %v", err)
	}
}
