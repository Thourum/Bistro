package ws

import (
	"encoding/json"
	"fmt"
	"log/slog"
)

// InboundMessage represents a message received from Electron.
type InboundMessage struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp string          `json:"timestamp"`
}

// MessageHandler processes an inbound WebSocket message and returns an optional response.
type MessageHandler func(msg InboundMessage) (interface{}, error)

// HandlerFunc is an alias for MessageHandler for backward compatibility.
type HandlerFunc = MessageHandler

// Dispatcher routes inbound WebSocket messages to registered handlers.
type Dispatcher struct {
	handlers map[string]MessageHandler
	logger   *slog.Logger
}

// NewDispatcher creates a new Dispatcher with the given logger.
func NewDispatcher(logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]MessageHandler),
		logger:   logger,
	}
}

// Register associates a message type with a handler function.
func (d *Dispatcher) Register(messageType string, handler MessageHandler) {
	d.handlers[messageType] = handler
}

// Dispatch parses raw bytes as an InboundMessage and calls the matching handler.
// Returns nil, nil when no handler is registered for the message type.
func (d *Dispatcher) Dispatch(raw []byte) (interface{}, error) {
	var msg InboundMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("invalid message format: %w", err)
	}

	handler, ok := d.handlers[msg.Type]
	if !ok {
		d.logger.Warn("unhandled message type", "type", msg.Type)
		return nil, nil
	}

	d.logger.Info("dispatching message", "type", msg.Type, "id", msg.ID)
	return handler(msg)
}
