package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/itterno/bistro/internal/engine"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/shell"
	"github.com/itterno/bistro/internal/tray"
	"github.com/wailsapp/wails/v3/pkg/application"
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
	deps.eng.OnSyncStatus(icon.Update)
	return icon
}
