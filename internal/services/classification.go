package services

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/zalando/go-keyring"
)

// ClassificationSettings contains only non-secret preferences. BYOK keys live in the keychain.
type ClassificationSettings struct {
	Mode           string `json:"mode"`
	DismissalScope string `json:"dismissal_scope"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Endpoint       string `json:"endpoint"`
}

const (
	classificationModeRules     = "rules"
	classificationModeRulesLLM  = "rules+llm"
	classificationModeLLMAll    = "llm-all"
	classificationModeServer    = "server"
	dismissalScopeLocal         = "local"
	dismissalScopeLocalProvider = "local+provider"
	defaultByokProvider         = "anthropic"
	byokKeyPrefix               = "byok."
)

// ClassificationState synchronizes preferences shared by sync, comments and dismissals.
type ClassificationState struct {
	mu       sync.RWMutex
	settings ClassificationSettings
}

// NewClassificationState starts in rules-only, local-only mode.
func NewClassificationState() *ClassificationState {
	return &ClassificationState{settings: ClassificationSettings{
		Mode: classificationModeRules, DismissalScope: dismissalScopeLocal, Provider: defaultByokProvider,
	}}
}

// Mode returns the live classification mode.
func (s *ClassificationState) Mode() string { return s.Snapshot().Mode }

// DismissalScope returns the live dismissal scope.
func (s *ClassificationState) DismissalScope() string { return s.Snapshot().DismissalScope }

// Snapshot returns a synchronized copy of the preferences.
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

// ByokKeyStore is the provider-scoped keychain seam, satisfied by keychain.TokenManager.
type ByokKeyStore interface {
	Store(provider, token string) error
	Delete(provider string) error
}

type ByokKeyRequest struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
}
type ByokProviderRequest struct {
	Provider string `json:"provider"`
}
type ByokKeyStatus struct {
	Provider string `json:"provider"`
	Present  bool   `json:"present"`
}

// Classification owns preference validation and secret-only keychain mutations.
type Classification struct {
	State        *ClassificationState
	Keys         ByokKeyStore
	OnKeyChanged func()
	Panics       PanicReporter
	Log          *slog.Logger
}

// SetSettings validates the whole update before replacing the shared state.
func (s *Classification) SetSettings(ctx context.Context, req ClassificationSettings) (ClassificationSettings, error) {
	return guard(s.Panics, "Classification.SetSettings", func() (ClassificationSettings, error) {
		if err := ctx.Err(); err != nil {
			return ClassificationSettings{}, err
		}
		if req.Mode != classificationModeRules && req.Mode != classificationModeRulesLLM && req.Mode != classificationModeLLMAll && req.Mode != classificationModeServer {
			return ClassificationSettings{}, Errorf(CodeInvalid, "invalid classification mode %q", req.Mode)
		}
		if req.DismissalScope != dismissalScopeLocal && req.DismissalScope != dismissalScopeLocalProvider {
			return ClassificationSettings{}, Errorf(CodeInvalid, "invalid dismissal scope %q", req.DismissalScope)
		}
		s.State.set(req)
		return req, nil
	})
}

// SetByokKey stores a key without exposing it to events, logs or responses.
func (s *Classification) SetByokKey(ctx context.Context, req ByokKeyRequest) (ByokKeyStatus, error) {
	return guard(s.Panics, "Classification.SetByokKey", func() (ByokKeyStatus, error) {
		if err := ctx.Err(); err != nil {
			return ByokKeyStatus{}, err
		}
		if req.Provider == "" || req.Key == "" {
			return ByokKeyStatus{}, Errorf(CodeInvalid, "provider and key are required")
		}
		if err := s.Keys.Store(byokKeyPrefix+req.Provider, req.Key); err != nil {
			if contextError(err) != nil {
				return ByokKeyStatus{}, err
			}
			// Keychain failures can contain key material, so never propagate or log them.
			return ByokKeyStatus{}, Errorf(CodeInternal, "failed to store key")
		}
		if s.OnKeyChanged != nil {
			s.OnKeyChanged()
		}
		return ByokKeyStatus{Provider: req.Provider, Present: true}, nil
	})
}

// DeleteByokKey treats a missing key as success, but never hides a failed deletion.
func (s *Classification) DeleteByokKey(ctx context.Context, req ByokProviderRequest) (ByokKeyStatus, error) {
	return guard(s.Panics, "Classification.DeleteByokKey", func() (ByokKeyStatus, error) {
		if err := ctx.Err(); err != nil {
			return ByokKeyStatus{}, err
		}
		if req.Provider == "" {
			return ByokKeyStatus{}, Errorf(CodeInvalid, "provider is required")
		}
		if err := s.Keys.Delete(byokKeyPrefix + req.Provider); err != nil {
			if contextError(err) != nil {
				return ByokKeyStatus{}, err
			}
			if !errors.Is(err, keyring.ErrNotFound) {
				return ByokKeyStatus{}, Errorf(CodeInternal, "failed to delete key")
			}
		}
		if s.OnKeyChanged != nil {
			s.OnKeyChanged()
		}
		return ByokKeyStatus{Provider: req.Provider, Present: false}, nil
	})
}
