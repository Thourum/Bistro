package logger

import (
	"testing"
)

func TestNewLogger(t *testing.T) {
	log := New()
	if log == nil {
		t.Error("expected logger, got nil")
	}
}

func TestLogInfo(t *testing.T) {
	log := New()
	log.Info("test message", "key", "value")
	// Just verify it doesn't panic
}

func TestLogError(t *testing.T) {
	log := New()
	log.Error("test error", "err", "test error message")
	// Just verify it doesn't panic
}
