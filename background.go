package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/itterno/bistro/internal/engine"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/shell"
	"github.com/itterno/bistro/internal/tray"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/dock"
)

type closeAction int

const (
	closeQuit closeAction = iota
	closeHide
)

type closeContext struct {
	isTrayReady bool
	isSmoke     bool
}

// Wails v3.0.0 only logs a failed StatusNotifierItem registration on Linux
// (systemtray_linux.go:659-666), so there is no readiness signal there and
// hiding could strand the app with no way back.
func isTrayReadyOn(goos string) bool { return goos != "linux" }

func decideCloseAction(c closeContext) closeAction {
	if c.isSmoke || !c.isTrayReady {
		return closeQuit
	}
	return closeHide
}

type visibilityActions struct {
	show func()
	hide func()
}

// Enqueue synchronously, but apply on one worker: independent goroutines can
// reverse Dock changes. Pending requests coalesce to the latest desired state.
// Cancellation stops idle work without making native shutdown wait for it.
func newVisibilityRequests(ctx context.Context, actions visibilityActions) visibilityActions {
	var mu sync.Mutex
	var pending func()
	wake := make(chan struct{}, 1)
	enqueue := func(action func()) {
		mu.Lock()
		defer mu.Unlock()
		if ctx.Err() != nil {
			return
		}
		pending = action
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-wake:
				mu.Lock()
				action := pending
				pending = nil
				mu.Unlock()
				if ctx.Err() != nil {
					return
				}
				if action != nil {
					action()
				}
			}
		}
	}()
	return visibilityActions{
		show: func() { enqueue(actions.show) },
		hide: func() { enqueue(actions.hide) },
	}
}

func newMainWindowVisibility(ctx context.Context, dk *dock.DockService) visibilityActions {
	dispatch := func(action func()) {
		application.InvokeAsync(func() {
			if ctx.Err() == nil {
				action()
			}
		})
	}
	return newVisibilityRequests(ctx, visibilityActions{
		show: func() {
			dk.ShowAppIcon()
			dispatch(focusMainWindow)
		},
		hide: func() {
			dk.HideAppIcon()
			dispatch(func() {
				if window, ok := application.Get().Window.GetByName("main"); ok {
					window.Hide()
				}
			})
		},
	})
}

// The worker keeps Dock calls off the main thread and orders window callbacks
// with them, so an earlier show cannot focus the window after a later hide.
func showMainWindow(visibility visibilityActions) {
	visibility.show()
}

func installCloseToTray(window *application.WebviewWindow, visibility visibilityActions) {
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		visibility.hide()
	})
}

func installCloseToQuit(window *application.WebviewWindow, quit func()) func() {
	return window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		quit()
	})
}

type trayDeps struct {
	app     *application.App
	eng     *engine.App
	set     *services.Set
	updates *shell.UpdateChecks
	show    func()
	ctx     context.Context
	log     *slog.Logger
}

func startTray(deps trayDeps) *tray.Tray {
	logIfFailed := func(action string, err error) {
		if err != nil {
			deps.log.Warn("tray action failed", "action", action, "err", err)
		}
	}
	icon := tray.New(deps.app, tray.Actions{
		Show: deps.show,
		SyncNow: func() {
			go func() {
				response, err := deps.set.Sync.Force(deps.ctx, services.ForceSyncRequest{})
				logIfFailed("sync", err)
				if err == nil && !response.Queued {
					deps.log.Info("manual sync already queued")
				}
			}()
		},
		CheckUpdates: func() {
			if !deps.updates.CheckNow() {
				deps.log.Info("update check unavailable")
			}
		},
		OpenLogs: func() {
			go func() { logIfFailed("open logs", deps.set.Shell.OpenLogFolder(deps.ctx)) }()
		},
		Quit: deps.app.Quit,
	}, tray.ViewOptions{IsDevBuild: version == "dev", Location: time.Local}, deps.eng.SyncStatus())
	deps.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		activateTrayStatus(deps.eng, icon.Update)
	})
	return icon
}

type trayStatusSource interface {
	SyncStatus() services.SyncStatus
	OnSyncStatus(func(services.SyncStatus))
}

// Register before reading the snapshot so startup transitions cannot be lost.
// Serialize reconciliation with callbacks so an older snapshot cannot win.
func activateTrayStatus(source trayStatusSource, apply func(services.SyncStatus)) {
	var delivery sync.Mutex
	delivery.Lock()
	defer delivery.Unlock()
	source.OnSyncStatus(func(status services.SyncStatus) {
		delivery.Lock()
		defer delivery.Unlock()
		apply(status)
	})
	apply(source.SyncStatus())
}
