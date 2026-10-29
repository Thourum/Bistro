package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/itterno/bistro/internal/classify/llm"
	"github.com/itterno/bistro/internal/keychain"
	"github.com/itterno/bistro/internal/services"
)

// slogPrintf adapts *slog.Logger to the llm.Logger Printf interface.
type slogPrintf struct{ log *slog.Logger }

// Printf forwards tier diagnostics to the structured application logger.
func (s slogPrintf) Printf(format string, v ...any) {
	s.log.Warn(fmt.Sprintf(format, v...))
}

// liveByokClient satisfies llm.ClassifierClient by constructing the right
// provider client per call from the LIVE settings + keychain key — so a
// provider/model/key change applies to the next call without restarts.
// The key never leaves the call path.
type liveByokClient struct {
	state  *services.ClassificationState
	tokens *keychain.TokenManager
}

// Classify uses the latest provider configuration and key for each request.
func (c *liveByokClient) Classify(ctx context.Context, comment llm.CommentInput) (string, int, error) {
	s := c.state.Snapshot()
	key, err := c.tokens.Get("byok." + s.Provider)
	if err != nil {
		// Missing key behaves like an auth failure: tier disables until a
		// key change calls ResetAuth.
		return "", 0, fmt.Errorf("no BYOK key for %s: %w", s.Provider, llm.ErrAuth)
	}
	if s.Provider == "anthropic" {
		return llm.NewAnthropicClient(s.Model, key, s.Endpoint).Classify(ctx, comment)
	}
	return llm.NewOpenAICompatibleClient(s.Provider, s.Model, key, s.Endpoint).Classify(ctx, comment)
}

func (a *App) prepareLLMTier(ctx context.Context) {
	// Tier no-ops outside LLM modes, so it can be wired unconditionally.
	a.llmTier = llm.NewTier(llm.TierConfig{
		DB:     a.db.DB,
		Client: &liveByokClient{state: a.classificationState, tokens: a.tokenManager},
		Mode:   a.classificationState.Mode,
		Settings: func() llm.SettingsSnapshot {
			s := a.classificationState.Snapshot()
			return llm.SettingsSnapshot{Provider: s.Provider, Model: s.Model}
		},
		Emit: func(eventType string, data map[string]any) {
			a.wsServer.Broadcast(wsEventMsg{Type: eventType, Data: data})
		},
		Log: slogPrintf{a.log},
	})
	a.llmKick = func(prID string) { a.llmTier.KickPR(ctx, prID) }
}
