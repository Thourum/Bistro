package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/itterno/bistro/internal/logging"
)

// InboundMessage represents a message received from Electron.
type InboundMessage struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp string          `json:"timestamp"`
}

// MessageHandler processes an inbound WebSocket message and returns an optional response.
// The context carries a request-scoped logger (see internal/logging).
type MessageHandler func(ctx context.Context, msg InboundMessage) (any, error)

// HandlerFunc is an alias for MessageHandler for backward compatibility.
type HandlerFunc = MessageHandler

// Dispatcher routes inbound WebSocket messages to registered handlers.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]MessageHandler
	logger   *slog.Logger
}

// NewDispatcher creates a new Dispatcher with the given logger.
func NewDispatcher(logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		handlers: make(map[string]MessageHandler),
		logger:   logger,
	}
}

// Register associates a message type with a handler function.
func (d *Dispatcher) Register(messageType string, handler MessageHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[messageType] = handler
}

// Dispatch parses raw bytes as an InboundMessage and calls the matching handler.
// Returns nil, nil when no handler is registered for the message type.
func (d *Dispatcher) Dispatch(ctx context.Context, raw []byte) (any, error) {
	var msg InboundMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		d.logger.Warn("invalid message format", "error", err, "payload_bytes", len(raw))
		return nil, fmt.Errorf("invalid message format: %w", err)
	}

	msg.Data = lowercaseGitHubIdentities(msg.Data)

	reqLogger := d.logger.With("request_id", msg.ID, "msg_type", msg.Type)
	reqCtx := logging.WithLogger(ctx, reqLogger)

	d.mu.RLock()
	handler, ok := d.handlers[msg.Type]
	d.mu.RUnlock()
	if !ok {
		reqLogger.Warn("unhandled message type")
		return nil, nil
	}

	// Payloads can contain OAuth credentials or BYOK API keys; log size only.
	reqLogger.Debug("dispatch start", "payload_bytes", len(raw))
	reqLogger.Info("dispatching message")

	start := time.Now()
	resp, err := handler(reqCtx, msg)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		reqLogger.Warn("dispatch err", "duration_ms", dur, "error", err)
		return resp, err
	}
	reqLogger.Debug("dispatch ok", "duration_ms", dur, "has_response", resp != nil)
	return resp, nil
}

// isIdentityField reports whether a request key holds repo or PR identities.
func isIdentityField(key string) bool {
	return key == "repo_id" || key == "pr_id" || key == "pr_ids"
}

const githubIdentityPrefix = "github:"

// lowercaseGitHubIdentities canonicalizes GitHub repo/PR IDs in a request
// payload once, so no handler sees GitHub's casing (the engine stores them
// lowercase; GitHub names are case-insensitive). Other providers' IDs are left
// alone. Payloads without such IDs are returned unchanged.
func lowercaseGitHubIdentities(data json.RawMessage) json.RawMessage {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber() // keep large numeric IDs exact on re-encode
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return data // handlers report malformed payloads themselves
	}
	if !lowercaseIdentityValues(payload, false) {
		return data
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return data
	}
	return canonical
}

// lowercaseIdentityValues rewrites identity strings in place and reports
// whether anything changed. isIdentity marks values under an identity key.
func lowercaseIdentityValues(value any, isIdentity bool) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if text, ok := child.(string); ok && isIdentityField(key) {
				if lowered := lowercaseGitHubID(text); lowered != text {
					typed[key] = lowered
					changed = true
				}
				continue
			}
			changed = lowercaseIdentityValues(child, isIdentityField(key)) || changed
		}
	case []any:
		for index, child := range typed {
			if text, ok := child.(string); ok && isIdentity {
				if lowered := lowercaseGitHubID(text); lowered != text {
					typed[index] = lowered
					changed = true
				}
				continue
			}
			changed = lowercaseIdentityValues(child, false) || changed
		}
	}
	return changed
}

func lowercaseGitHubID(id string) string {
	if !strings.HasPrefix(strings.ToLower(id), githubIdentityPrefix) {
		return id
	}
	return strings.ToLower(id)
}
