package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAppRegistersRepoLocationHandlers(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cacheRoot := filepath.Join(t.TempDir(), "bistro-cache")
	t.Setenv("BISTRO_REPO_DIR", cacheRoot)

	app := NewWithLogger(0, filepath.Join(t.TempDir(), "sync.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	app.SetAutoSync(false)
	app.SetReadyWriter(io.Discard)

	runErr := make(chan error, 1)
	go func() {
		runErr <- app.Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-runErr:
			if err != nil {
				t.Errorf("Run returned error: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Errorf("timed out waiting for app shutdown")
		}
	})

	requireAppPort(t, app)

	testCases := []struct {
		name        string
		messageType string
		data        map[string]string
	}{
		{name: "VALIDATE_DEV_FOLDER", messageType: "VALIDATE_DEV_FOLDER", data: map[string]string{"path": ""}},
		{name: "SCAN_DEV_FOLDER", messageType: "SCAN_DEV_FOLDER", data: map[string]string{"path": ""}},
		{name: "ADD_REPO_BY_URL", messageType: "ADD_REPO_BY_URL", data: map[string]string{"url": "not a url"}},
		{name: "REMOVE_REPO_PREFLIGHT", messageType: "REMOVE_REPO_PREFLIGHT", data: map[string]string{"repo_id": "github:acme/api"}},
		{name: "GET_CACHE_STATS", messageType: "GET_CACHE_STATS", data: map[string]string{}},
		{name: "PRUNE_CACHE", messageType: "PRUNE_CACHE", data: map[string]string{}},
		{name: "CLEAR_CACHE", messageType: "CLEAR_CACHE", data: map[string]string{}},
		{name: "DELETE_CACHE_REPO", messageType: "DELETE_CACHE_REPO", data: map[string]string{"repo_id": "github:acme/api"}},
		{name: "RECOVER_REPO_PATH", messageType: "RECOVER_REPO_PATH", data: map[string]string{"repo_id": "github:acme/api", "path": ""}},
		{name: "RECOVER_REPO_TO_CACHE", messageType: "RECOVER_REPO_TO_CACHE", data: map[string]string{"repo_id": "github:acme/api"}},
		{name: "REMOVE_REPO", messageType: "REMOVE_REPO", data: map[string]string{"repo_id": "github:acme/api"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			// Act
			response, err := repoLocationSmokeRequest(context.Background(), app.wsServer.Port(), repoLocationSmokeMessage(t, testCase.messageType, testCase.data))

			// Assert
			if err != nil {
				t.Fatalf("Dispatch returned error: %v", err)
			}
			if response == nil {
				t.Fatalf("expected %s handler response", testCase.messageType)
			}
		})
	}
}

func requireAppPort(t *testing.T, app *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if app.wsServer.Port() != 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for app websocket port")
}

func repoLocationSmokeRequest(ctx context.Context, port int, message []byte) (json.RawMessage, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		return nil, fmt.Errorf("dial websocket: %w", err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
		return nil, fmt.Errorf("write websocket message: %w", err)
	}
	_, response, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read websocket response: %w", err)
	}
	return response, nil
}

func repoLocationSmokeMessage(t *testing.T, messageType string, data map[string]string) []byte {
	t.Helper()
	payload, err := json.Marshal(struct {
		ID        string            `json:"id"`
		Type      string            `json:"type"`
		Data      map[string]string `json:"data"`
		Timestamp string            `json:"timestamp"`
	}{
		ID:        "test-" + messageType,
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	return payload
}
