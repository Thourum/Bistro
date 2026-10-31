package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/tray"
)

func TestDecideCloseAction(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  closeContext
		want closeAction
	}{
		{"tray ready hides", closeContext{isTrayReady: true}, closeHide},
		{"no tray quits", closeContext{isTrayReady: false}, closeQuit},
		{"smoke never intercepts", closeContext{isTrayReady: true, isSmoke: true}, closeQuit},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := decideCloseAction(test.ctx); got != test.want {
				t.Fatalf("decideCloseAction(%+v) = %v, want %v", test.ctx, got, test.want)
			}
		})
	}
	if isTrayReadyOn("linux") || !isTrayReadyOn("darwin") || !isTrayReadyOn("windows") {
		t.Fatal("tray readiness must be false only on linux")
	}
}

type startupStatusSource struct {
	status   services.SyncStatus
	listener func(services.SyncStatus)
	read     func() services.SyncStatus
}

func (s *startupStatusSource) SyncStatus() services.SyncStatus {
	if s.read != nil {
		return s.read()
	}
	return s.status
}

func (s *startupStatusSource) OnSyncStatus(listener func(services.SyncStatus)) {
	s.listener = listener
}

func TestTrayStatusActivationReconcilesAndOrdersTransitions(t *testing.T) {
	// The tray was constructed while syncing; completion happened before Run.
	view := tray.BuildView(services.SyncStatus{State: services.SyncStateSyncing}, tray.ViewOptions{})
	source := &startupStatusSource{status: services.SyncStatus{State: services.SyncStateUpToDate}}
	var applied []tray.Icon
	apply := func(status services.SyncStatus) {
		view = tray.BuildView(status, tray.ViewOptions{})
		applied = append(applied, view.Icon)
	}
	activateTrayStatus(source, apply)
	if view.Icon != tray.IconIdle {
		t.Fatalf("startup completion was lost: %s", view.Icon)
	}

	// A transition published during reconciliation must follow its snapshot.
	callbackDone := make(chan struct{})
	source.read = func() services.SyncStatus {
		go func() {
			source.listener(services.SyncStatus{State: services.SyncStateError, ErrorCode: "SYNC_FAILED"})
			close(callbackDone)
		}()
		const reconciliationObservation = 20 * time.Millisecond
		select {
		case <-callbackDone:
			t.Error("transition bypassed the reconciliation ordering guard")
		case <-time.After(reconciliationObservation):
		}
		return source.status
	}
	activateTrayStatus(source, apply)
	const callbackDeadline = 3 * time.Second
	select {
	case <-callbackDone:
	case <-time.After(callbackDeadline):
		t.Fatal("status transition deadlocked during reconciliation")
	}
	if view.Icon != tray.IconAttention {
		t.Fatalf("reconciliation overwrote the later error: %s", view.Icon)
	}
	if want := []tray.Icon{tray.IconIdle, tray.IconIdle, tray.IconAttention}; !reflect.DeepEqual(applied, want) {
		t.Fatalf("status presentation order = %v, want %v", applied, want)
	}
}
