package ws

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestWebSocketLogsDoNotContainMessageData(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	server := NewServerWithLogger(logger)
	server.RegisterHandler("SET_BYOK_KEY", func(context.Context, InboundMessage) (any, error) {
		return nil, nil
	})
	secret := "sensitive-key-test-value"
	message, err := json.Marshal(InboundMessage{Type: "SET_BYOK_KEY", Data: json.RawMessage(`{"key":"` + secret + `"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.dispatcher.Dispatch(context.Background(), message); err != nil {
		t.Fatal(err)
	}
	server.Broadcast(map[string]any{"type": "SET_BYOK_KEY", "data": map[string]string{"key": secret}})
	if strings.Contains(output.String(), secret) {
		t.Fatal("websocket debug log contains message data")
	}
}

func TestNewServer(t *testing.T) {
	server := NewServer()
	if server == nil {
		t.Error("expected server, got nil")
	}
}

func TestServerPort(t *testing.T) {
	server := NewServer()

	if got := server.Port(); got != 0 {
		t.Errorf("Port() before Start = %d, want 0", got)
	}

	if err := server.Start(":0"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer server.Stop() //nolint:errcheck

	port := server.Port()
	if port <= 0 || port > 65535 {
		t.Errorf("Port() after Start(\":0\") = %d, want a valid TCP port", port)
	}
}

func TestStartServer(t *testing.T) {
	server := NewServer()
	startErr := make(chan error, 1)
	go func() {
		err := server.Start(":0") // Random port
		if err != nil && err.Error() != "server stopped" {
			startErr <- err
		} else {
			startErr <- nil
		}
	}()

	time.Sleep(100 * time.Millisecond)

	err := server.Stop()
	if err != nil {
		t.Errorf("failed to stop server: %v", err)
	}

	if goroutineErr := <-startErr; goroutineErr != nil {
		t.Errorf("unexpected error from server goroutine: %v", goroutineErr)
	}
}
