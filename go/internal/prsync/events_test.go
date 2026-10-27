package prsync

import (
	"testing"
	"time"

	"github.com/itterno/bistro/internal/events"
)

func TestSyncEventCreation(t *testing.T) {
	event := &events.Event{
		Type:      "sync_started",
		Timestamp: time.Now(),
	}

	if event.Type != "sync_started" {
		t.Errorf("expected type sync_started, got %s", event.Type)
	}
}

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

func TestProgressEvent(t *testing.T) {
	event := events.NewProgressEvent("fetching_prs", 5, 50)
	if event.Type != "sync_progress" {
		t.Errorf("expected type sync_progress, got %s", event.Type)
	}
}

func TestCompleteEvent(t *testing.T) {
	event := events.NewCompleteEvent(42, 3, 5, 0, 2*time.Second)
	if event.Type != "sync_complete" {
		t.Errorf("expected type sync_complete, got %s", event.Type)
	}
}
