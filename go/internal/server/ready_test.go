package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmitReady(t *testing.T) {
	var buf bytes.Buffer

	if err := emitReady(&buf, 54321); err != nil {
		t.Fatalf("emitReady failed: %v", err)
	}

	line := buf.String()
	if !strings.HasSuffix(line, "\n") {
		t.Errorf("ready signal must end with newline, got %q", line)
	}

	var signal struct {
		Event  string `json:"event"`
		WSPort int    `json:"wsPort"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &signal); err != nil {
		t.Fatalf("ready signal is not valid JSON: %v (line: %q)", err, line)
	}
	if signal.Event != "ready" {
		t.Errorf("event = %q, want %q", signal.Event, "ready")
	}
	if signal.WSPort != 54321 {
		t.Errorf("wsPort = %d, want 54321", signal.WSPort)
	}
}

func TestRunFailsWhenReadySignalCannotBeWritten(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app := NewWithLogger(0, filepath.Join(t.TempDir(), "sync.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	app.SetReadyWriter(failingWriter{})
	app.SetAutoSync(false)
	if err := app.Run(ctx); !errors.Is(err, errWriteFailed) {
		t.Fatalf("Run() = %v, want ready writer error", err)
	}
}

func TestEmitReadyWriteError(t *testing.T) {
	if err := emitReady(failingWriter{}, 8080); err == nil {
		t.Error("expected error from failing writer, got nil")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errWriteFailed
}

var errWriteFailed = &writeError{}

type writeError struct{}

func (*writeError) Error() string { return "write failed" }
