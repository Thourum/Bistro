package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
