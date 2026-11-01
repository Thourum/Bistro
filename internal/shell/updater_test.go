package shell

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

func TestUpdateChecksRejectUnavailableChecks(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, checks := range map[string]*UpdateChecks{
		"disabled":  {},
		"stopping":  {checkAndInstall: func(context.Context) error { return nil }, ctx: context.Background(), isStopping: true},
		"cancelled": {checkAndInstall: func(context.Context) error { return nil }, ctx: cancelled},
	} {
		t.Run(name, func(t *testing.T) {
			if checks.CheckNow() {
				t.Fatal("unavailable updater accepted a manual check")
			}
		})
	}
}

func TestUpdateChecksIgnoreConcurrentAndReadyRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	var state atomic.Value
	state.Store(updater.StateIdle)
	started := make(chan struct{})
	checks := &UpdateChecks{
		ctx: ctx, cancel: cancel, log: slog.Default(),
		state: func() updater.State { return state.Load().(updater.State) },
		checkAndInstall: func(context.Context) error {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-ctx.Done()
			return ctx.Err()
		},
	}
	t.Cleanup(checks.Stop)
	if !checks.CheckNow() {
		t.Fatal("initial manual check rejected")
	}
	select {
	case <-started:
	case <-time.After(testDeadline):
		t.Fatal("initial check did not start")
	}
	if !checks.CheckNow() {
		t.Fatal("concurrent request reported unavailable")
	}
	cancel()
	checks.manual.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent requests started %d checks, want 1", got)
	}

	// A completed install still owns a session waiting for Restart.
	checks.ctx = context.Background()
	state.Store(updater.StateReady)
	if !checks.CheckNow() {
		t.Fatal("ready request reported unavailable")
	}
	checks.manual.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("ready request started another check: %d calls", got)
	}
}

func TestUpdateChecksStopCancelsAndDrainsManualChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	periodicDone := make(chan struct{})
	checks := &UpdateChecks{checkAndInstall: func(context.Context) error { return nil }, ctx: ctx, cancel: cancel, done: periodicDone}
	checks.manual.Add(1)
	stopped := make(chan struct{})
	go func() {
		checks.Stop()
		close(stopped)
	}()
	select {
	case <-ctx.Done():
	case <-time.After(testDeadline):
		t.Fatal("Stop did not cancel checks")
	}
	if checks.CheckNow() {
		t.Error("shutdown accepted a new manual check")
	}
	close(periodicDone)
	const drainObservationWindow = 20 * time.Millisecond
	select {
	case <-stopped:
		t.Error("Stop returned with a manual check still running")
	case <-time.After(drainObservationWindow):
	}
	checks.manual.Done()
	select {
	case <-stopped:
	case <-time.After(testDeadline):
		t.Fatal("Stop did not finish after the manual check drained")
	}
}
