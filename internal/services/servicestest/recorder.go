package servicestest

import (
	"slices"
	"sync"
)

// Event preserves the name and payload emitted by a service.
type Event struct {
	Name string
	Data any
}

// Recorder captures service events safely across concurrent calls.
type Recorder struct {
	mu     sync.Mutex
	events []Event
}

// Emit appends events in the order they acquire the recorder lock.
func (r *Recorder) Emit(name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, Event{Name: name, Data: data})
}

// Events returns a snapshot so callers cannot mutate the recorder's slice.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.events)
}

// Named preserves emission order among events with the requested name.
func (r *Recorder) Named(name string) []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	var payloads []any
	for _, event := range r.events {
		if event.Name == name {
			payloads = append(payloads, event.Data)
		}
	}
	return payloads
}
