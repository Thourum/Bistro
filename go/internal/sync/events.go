package sync

import (
	"time"

	"github.com/samuelasvanyi/bistro/internal/events"
)

// SyncEvent is an alias for events.Event for backward compatibility within this package.
type SyncEvent = events.Event

// EventEmitter is an alias for events.EventEmitter for backward compatibility.
type EventEmitter = events.EventEmitter

// NewEventEmitter creates a new EventEmitter.
func NewEventEmitter() *EventEmitter {
	return events.NewEventEmitter()
}

// NewProgressEvent creates a sync progress event.
func NewProgressEvent(stage string, current, total int) *SyncEvent {
	return events.NewProgressEvent(stage, current, total)
}

// NewCompleteEvent creates a sync completion event.
func NewCompleteEvent(prCount, newPRs, updatedPRs, errs int, duration time.Duration) *SyncEvent {
	return events.NewCompleteEvent(prCount, newPRs, updatedPRs, errs, duration)
}

// NewErrorEvent creates a sync error event.
func NewErrorEvent(message string) *SyncEvent {
	return events.NewErrorEvent(message)
}
