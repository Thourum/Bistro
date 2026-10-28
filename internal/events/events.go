// Package events provides shared event types and the EventEmitter broadcaster
// used by both the prsync and actions packages.
package events

import (
	"sync"
	"time"
)

// Keep a short burst of events without blocking a sync on a slow UI client.
const eventSubscriberBuffer = 10

// Event represents a system event emitted during sync or action execution.
type Event struct {
	Type      string         `json:"type"`
	Data      map[string]any `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
}

// EventEmitter broadcasts events to multiple subscribers.
type EventEmitter struct {
	mu          sync.RWMutex
	subscribers map[<-chan *Event]chan<- *Event
}

// NewEventEmitter creates a new EventEmitter.
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		subscribers: make(map[<-chan *Event]chan<- *Event),
	}
}

// Subscribe creates a new buffered event channel and registers it.
func (e *EventEmitter) Subscribe() <-chan *Event {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan *Event, eventSubscriberBuffer)
	e.subscribers[ch] = ch
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (e *EventEmitter) Unsubscribe(ch <-chan *Event) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if send, ok := e.subscribers[ch]; ok {
		close(send)
		delete(e.subscribers, ch)
	}
}

// Emit broadcasts an event to all subscribers, dropping events on full channels.
func (e *EventEmitter) Emit(event *Event) {
	if event == nil {
		return
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, ch := range e.subscribers {
		select {
		case ch <- event:
		default:
			// Channel full, skip to avoid blocking
		}
	}
}

// NewProgressEvent creates a progress event.
func NewProgressEvent(stage string, current, total int) *Event {
	return &Event{
		Type: "sync_progress",
		Data: map[string]any{
			"stage":   stage,
			"current": current,
			"total":   total,
		},
		Timestamp: time.Now(),
	}
}

// NewCompleteEvent creates a sync completion event.
func NewCompleteEvent(prCount, newPRs, updatedPRs, errors int, duration time.Duration) *Event {
	return &Event{
		Type: "sync_complete",
		Data: map[string]any{
			"pr_count":    prCount,
			"new_prs":     newPRs,
			"updated_prs": updatedPRs,
			"errors":      errors,
			"duration_ms": duration.Milliseconds(),
		},
		Timestamp: time.Now(),
	}
}

// NewPRMetadataChangedEvent fires when a PR's title, state, or draft flag changes.
func NewPRMetadataChangedEvent(prID, owner, repo string, number int, field, oldVal, newVal string) *Event {
	return &Event{
		Type: "PR_METADATA_CHANGED",
		Data: map[string]any{
			"pr_id":  prID,
			"owner":  owner,
			"repo":   repo,
			"number": number,
			"field":  field,
			"old":    oldVal,
			"new":    newVal,
		},
		Timestamp: time.Now(),
	}
}

// NewCIStatusChangedEvent fires when a PR's CI status changes.
func NewCIStatusChangedEvent(prID, owner, repo string, number int, oldStatus, newStatus string) *Event {
	return &Event{
		Type: "CI_STATUS_CHANGED",
		Data: map[string]any{
			"pr_id":      prID,
			"owner":      owner,
			"repo":       repo,
			"number":     number,
			"old_status": oldStatus,
			"new_status": newStatus,
		},
		Timestamp: time.Now(),
	}
}

// NewErrorEvent creates an error event.
func NewErrorEvent(message string) *Event {
	return &Event{
		Type: "sync_error",
		Data: map[string]any{
			"error": message,
		},
		Timestamp: time.Now(),
	}
}
