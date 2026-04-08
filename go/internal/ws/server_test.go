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
