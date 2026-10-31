package shell

import (
	"context"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestUpdateChecksRejectUnavailableChecks(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, checks := range map[string]*UpdateChecks{
		"disabled":  {},
		"stopping":  {app: &application.App{}, ctx: context.Background(), isStopping: true},
		"cancelled": {app: &application.App{}, ctx: cancelled},
	} {
		t.Run(name, func(t *testing.T) {
			if checks.CheckNow() {
				t.Fatal("unavailable updater accepted a manual check")
			}
		})
	}
}

func TestUpdateChecksStopCancelsAndDrainsManualChecks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	periodicDone := make(chan struct{})
	checks := &UpdateChecks{app: &application.App{}, ctx: ctx, cancel: cancel, done: periodicDone}
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
