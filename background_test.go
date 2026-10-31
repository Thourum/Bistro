package main

import (
	"context"
	"reflect"
	"sync/atomic"
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

func TestVisibilityRequestsPreserveLatestState(t *testing.T) {
	for _, firstIsShow := range []bool{false, true} {
		name := "hide then show"
		if firstIsShow {
			name = "show then hide"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			started := make(chan struct{})
			release := make(chan struct{})
			defer close(release)
			finished := make(chan struct{}, 2)
			var iconShown atomic.Bool
			var windowShown atomic.Bool
			apply := func(action struct{ isShow bool }) {
				if action.isShow == firstIsShow {
					close(started)
					select {
					case <-release:
					case <-ctx.Done():
						return
					}
				}
				iconShown.Store(action.isShow)
				windowShown.Store(action.isShow)
				finished <- struct{}{}
			}
			requests := newVisibilityRequests(ctx, visibilityActions{
				show: func() { apply(struct{ isShow bool }{isShow: true}) },
				hide: func() { apply(struct{ isShow bool }{isShow: false}) },
			})
			first, second := requests.hide, requests.show
			if firstIsShow {
				first, second = second, first
			}
			const deadline = 3 * time.Second
			first()
			select {
			case <-started:
			case <-time.After(deadline):
				t.Fatal("first visibility request did not start")
			}
			second()
			// Give an incorrectly independent second worker time to overtake
			// the blocked first request, then check the resulting final state.
			remaining := 2
			const orderingObservation = 20 * time.Millisecond
			select {
			case <-finished:
				remaining--
			case <-time.After(orderingObservation):
			}
			release <- struct{}{}
			for range remaining {
				select {
				case <-finished:
				case <-time.After(deadline):
					t.Fatal("visibility requests did not finish")
				}
			}
			if wantShown := !firstIsShow; iconShown.Load() != wantShown || windowShown.Load() != wantShown {
				t.Fatalf("final icon shown=%v, window shown=%v; want both %v", iconShown.Load(), windowShown.Load(), wantShown)
			}
		})
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
