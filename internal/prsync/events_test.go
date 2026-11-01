package prsync

import (
	"testing"
	"time"

	"github.com/itterno/bistro/internal/events"
)

func TestEventEmitterCreation(t *testing.T) {
	emitter := events.NewEventEmitter()
	if emitter == nil {
		t.Error("expected non-nil emitter")
	}
}

func TestEventEmitterSubscribe(t *testing.T) {
	emitter := events.NewEventEmitter()
	ch := emitter.Subscribe()

	if ch == nil {
		t.Error("expected non-nil channel")
	}

	emitter.Unsubscribe(ch)
}

func TestEventEmitterEmit(t *testing.T) {
	emitter := events.NewEventEmitter()
	ch := emitter.Subscribe()
	defer emitter.Unsubscribe(ch)

	event := &events.Event{Type: "test_event"}
	go emitter.Emit(event)

	select {
	case received := <-ch:
		if received.Type != "test_event" {
			t.Errorf("expected test_event, got %s", received.Type)
		}
	case <-time.After(1 * time.Second):
		t.Error("event not received in time")
	}
}
