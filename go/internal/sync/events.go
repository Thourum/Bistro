package sync

import (
	"sync"
	"time"
)

// SyncEvent represents a sync progress or completion event
type SyncEvent struct {
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// EventEmitter broadcasts events to multiple subscribers
type EventEmitter struct {
	mu          sync.RWMutex
	subscribers map[<-chan *SyncEvent]chan<- *SyncEvent
}

// NewEventEmitter creates a new event emitter
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		subscribers: make(map[<-chan *SyncEvent]chan<- *SyncEvent),
	}
}

// Subscribe creates a new event channel and registers it
func (e *EventEmitter) Subscribe() <-chan *SyncEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan *SyncEvent, 10) // Buffered to avoid blocking
	e.subscribers[ch] = ch
	return ch
}

// Unsubscribe removes a subscriber and closes its channel
func (e *EventEmitter) Unsubscribe(ch <-chan *SyncEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if send, ok := e.subscribers[ch]; ok {
		close(send)
		delete(e.subscribers, ch)
	}
}

// Emit broadcasts an event to all subscribers
func (e *EventEmitter) Emit(event *SyncEvent) {
	if event == nil {
		return
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, ch := range e.subscribers {
		select {
		case ch <- event:
			// Event sent
		default:
			// Channel full, skip (don't block)
		}
	}
}

// NewProgressEvent creates a sync progress event
func NewProgressEvent(stage string, current, total int) *SyncEvent {
	return &SyncEvent{
		Type: "sync_progress",
		Data: map[string]interface{}{
			"stage":   stage,
			"current": current,
			"total":   total,
		},
		Timestamp: time.Now(),
	}
}

// NewCompleteEvent creates a sync completion event
func NewCompleteEvent(prCount, newPRs, updatedPRs, errors int, duration time.Duration) *SyncEvent {
	return &SyncEvent{
		Type: "sync_complete",
		Data: map[string]interface{}{
			"pr_count":    prCount,
			"new_prs":     newPRs,
			"updated_prs": updatedPRs,
			"errors":      errors,
			"duration_ms": duration.Milliseconds(),
		},
		Timestamp: time.Now(),
	}
}

// NewErrorEvent creates a sync error event
func NewErrorEvent(message string) *SyncEvent {
	return &SyncEvent{
		Type: "sync_error",
		Data: map[string]interface{}{
			"error": message,
		},
		Timestamp: time.Now(),
	}
}
