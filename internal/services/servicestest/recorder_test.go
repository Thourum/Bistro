package servicestest

import (
	"slices"
	"sync"
	"testing"
)

func TestRecorderFiltersInOrderAndReturnsSnapshots(t *testing.T) {
	var recorder Recorder
	recorder.Emit("changed", "first")
	recorder.Emit("other", "ignored")
	recorder.Emit("changed", "second")
	if got := recorder.Named("changed"); !slices.Equal(got, []any{"first", "second"}) {
		t.Fatalf("got %v, want matching payloads in order", got)
	}
	if got := recorder.Named("missing"); len(got) != 0 {
		t.Fatalf("got %v for missing name", got)
	}
	events := recorder.Events()
	want := []Event{{Name: "changed", Data: "first"}, {Name: "other", Data: "ignored"}, {Name: "changed", Data: "second"}}
	if !slices.Equal(events, want) {
		t.Fatalf("got %v, want %v", events, want)
	}
	events[0].Name = "mutated"
	if got := recorder.Events(); !slices.Equal(got, want) {
		t.Fatalf("snapshot mutation changed recorder: %v", got)
	}
}

func TestRecorderConcurrentAccess(t *testing.T) {
	var recorder Recorder
	const writers = 8
	var group sync.WaitGroup
	for i := range writers {
		group.Go(func() {
			recorder.Emit("changed", i)
			recorder.Events()
			recorder.Named("changed")
		})
	}
	group.Wait()
	payloads := recorder.Named("changed")
	if len(payloads) != writers {
		t.Fatalf("got %d events, want %d", len(payloads), writers)
	}
	seen := make(map[any]bool)
	for _, payload := range payloads {
		seen[payload] = true
	}
	for i := range writers {
		if !seen[i] {
			t.Errorf("missing payload %d", i)
		}
	}
}
