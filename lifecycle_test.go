package main

import (
	"reflect"
	"testing"
	"time"
)

func TestCleanupRunsAllResourcesOnceInReverseOrder(t *testing.T) {
	var released []string
	cleanup := &appCleanup{}
	for _, resource := range []string{"smoke", "log", "sentry", "telemetry", "engine", "watcher"} {
		cleanup.add(func() { released = append(released, resource) })
	}
	cleanup.close() // Native shutdown, without a returning Run loop.
	cleanup.close() // The deferred fallback must not close resources twice.
	if !reflect.DeepEqual(released, []string{"watcher", "engine", "telemetry", "sentry", "log", "smoke"}) {
		t.Fatalf("cleanup order = %v", released)
	}
}

func TestQuitKeepsNativeThreadAvailableUntilUpdaterDrains(t *testing.T) {
	const deadline = 3 * time.Second
	nativeWork := make(chan struct{})
	drained := make(chan struct{})
	quit := make(chan struct{})
	coordinator := &quitCoordinator{
		stop: func() { close(nativeWork); <-drained },
		quit: func() { close(quit) },
	}
	if coordinator.shouldQuit() {
		t.Fatal("quit allowed before updater drained")
	}
	select {
	case <-nativeWork:
	case <-time.After(deadline):
		t.Fatal("native thread blocked joining updater")
	}
	if coordinator.shouldQuit() {
		t.Fatal("repeated quit allowed while draining")
	}
	close(drained) // The native thread can now service the outstanding UI work.
	select {
	case <-quit:
	case <-time.After(deadline):
		t.Fatal("quit was not retried after drain")
	}
	if !coordinator.shouldQuit() {
		t.Fatal("quit denied after updater drained")
	}
}
