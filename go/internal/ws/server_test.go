package ws

import (
	"testing"
	"time"
)

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
