package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/zalando/go-keyring"

	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/ws"
)

// ClassificationSettings is the live, non-secret classification config pushed
// from Electron via SET_CLASSIFICATION_SETTINGS. The API key is NEVER part of
// this struct — it travels only through SET_BYOK_KEY into the OS keychain.
type ClassificationSettings struct {
	Mode           string `json:"mode"`
	DismissalScope string `json:"dismissal_scope"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Endpoint       string `json:"endpoint"`
}

func isValidClassificationMode(mode string) bool {
	// server is a hidden Team-plan stub: accepted and stored, but the LLM
	// tier treats it as "rules".
	switch mode {
	case "rules", "rules+llm", "llm-all", "server":
		return true
	default:
		return false
	}
}

func isValidDismissalScope(scope string) bool {
	return scope == "local" || scope == "local+provider"
}

// ClassificationState holds the live settings behind a mutex — the writer,
// the comments handler (via the engine provider's mode getter), and the
// dismiss handlers all read it concurrently.
type ClassificationState struct {
	mu       sync.RWMutex
	settings ClassificationSettings
}

// NewClassificationState starts at the defaults: rules-only, local-only.
func NewClassificationState() *ClassificationState {
	return &ClassificationState{
		settings: ClassificationSettings{
			Mode:           "rules",
			DismissalScope: "local",
			Provider:       "anthropic",
		},
	}
}

// Mode returns the live classification mode ("rules" | "rules+llm" | "llm-all" | "server").
func (s *ClassificationState) Mode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.Mode
}

// DismissalScope returns the live dismissal scope ("local" | "local+provider").
func (s *ClassificationState) DismissalScope() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.DismissalScope
}

// Snapshot returns a copy of the full settings.
func (s *ClassificationState) Snapshot() ClassificationSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *ClassificationState) set(next ClassificationSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = next
}

// NewSetClassificationSettingsHandler returns the SET_CLASSIFICATION_SETTINGS
// handler: validate, store, ack with CLASSIFICATION_SETTINGS_UPDATED. Invalid
// values reject the whole message and leave the state unchanged.
func NewSetClassificationSettingsHandler(state *ClassificationState, log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req ClassificationSettings
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid SET_CLASSIFICATION_SETTINGS payload: %w", err)
		}
		if !isValidClassificationMode(req.Mode) {
			return nil, fmt.Errorf("invalid classification mode %q", req.Mode)
		}
		if !isValidDismissalScope(req.DismissalScope) {
			return nil, fmt.Errorf("invalid dismissal scope %q", req.DismissalScope)
		}

		state.set(req)
		logger.Info("classification settings updated",
			"mode", req.Mode, "dismissal_scope", req.DismissalScope,
			"provider", req.Provider, "model", req.Model)

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "CLASSIFICATION_SETTINGS_UPDATED",
			"data": map[string]interface{}{
				"mode":            req.Mode,
				"dismissal_scope": req.DismissalScope,
				"provider":        req.Provider,
				"model":           req.Model,
				"endpoint":        req.Endpoint,
			},
		}, nil
	}
}

// ByokKeyStore is the keychain seam for BYOK API keys. Satisfied by
// keychain.TokenManager (Store/Delete by provider-scoped key).
type ByokKeyStore interface {
	Store(provider, token string) error
	Delete(provider string) error
}

// byokKeyName namespaces BYOK keys away from OAuth tokens in the keychain:
// bistro-sync / byok.<provider>.
func byokKeyName(provider string) string { return "byok." + provider }

type byokKeyRequest struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
}

// NewSetByokKeyHandler stores a BYOK API key in the OS keychain. The key is
// NEVER echoed back, never logged, never persisted anywhere else.
func NewSetByokKeyHandler(store ByokKeyStore, onKeyChanged func(), log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req byokKeyRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid SET_BYOK_KEY payload: %w", err)
		}
		if req.Provider == "" || req.Key == "" {
			return nil, fmt.Errorf("SET_BYOK_KEY requires provider and key")
		}

		if err := store.Store(byokKeyName(req.Provider), req.Key); err != nil {
			// Keychain errors may contain secret metadata; log once and return a safe message.
			logger.Error("BYOK key store failed", "provider", req.Provider, "err", err)
			return nil, fmt.Errorf("failed to store key for %s", req.Provider)
		}
		if onKeyChanged != nil {
			onKeyChanged()
		}
		logger.Info("BYOK key stored", "provider", req.Provider)

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "BYOK_KEY_UPDATED",
			"data": map[string]interface{}{"provider": req.Provider, "present": true},
		}, nil
	}
}

// NewDeleteByokKeyHandler removes a BYOK API key from the OS keychain.
func NewDeleteByokKeyHandler(store ByokKeyStore, onKeyChanged func(), log *slog.Logger) ws.MessageHandler {
	return func(ctx context.Context, msg ws.InboundMessage) (interface{}, error) {
		logger := logging.FromContext(ctx)
		if log != nil {
			logger = log.With("request_id", msg.ID, "msg_type", msg.Type)
		}

		var req byokKeyRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil, fmt.Errorf("invalid DELETE_BYOK_KEY payload: %w", err)
		}
		if req.Provider == "" {
			return nil, fmt.Errorf("DELETE_BYOK_KEY requires provider")
		}

		// A missing key is idempotent success; any other failure must surface —
		// reporting present=false while credential material remains in the
		// keychain would mislead the user. No key material in errors.
		if err := store.Delete(byokKeyName(req.Provider)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			logger.Error("BYOK key delete failed", "provider", req.Provider, "err", err)
			return nil, fmt.Errorf("failed to delete key for %s", req.Provider)
		}
		if onKeyChanged != nil {
			onKeyChanged()
		}
		logger.Info("BYOK key deleted", "provider", req.Provider)

		return map[string]interface{}{
			"id":   msg.ID,
			"type": "BYOK_KEY_UPDATED",
			"data": map[string]interface{}{"provider": req.Provider, "present": false},
		}, nil
	}
}
