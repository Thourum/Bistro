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
)

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
