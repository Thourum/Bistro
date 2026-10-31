package main

import (
	"log/slog"
	"sync"
	"time"

	"github.com/itterno/bistro/internal/shell"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const windowSaveDebounce = 500 * time.Millisecond

func newMainWindow(app *application.App, settings *shell.SettingsStore, log *slog.Logger) *application.WebviewWindow {
	bounds := settings.Get().Window
	options := application.WebviewWindowOptions{
		Name: "main", Title: "Bistro", URL: "/", Width: 1600, Height: 900,
		MinWidth: 960, MinHeight: 600, InitialPosition: application.WindowCentered,
		UseApplicationMenu: true,
		Mac:                application.MacWindow{TitleBar: application.MacTitleBarHidden, InvisibleTitleBarHeight: 32},
	}
	if bounds.Width != 0 {
		options.Width, options.Height = bounds.Width, bounds.Height
		options.X, options.Y = bounds.X, bounds.Y
		options.InitialPosition = application.WindowXY
	}
	if bounds.Maximised {
		options.StartState = application.WindowStateMaximised
	}
	window := app.Window.NewWithOptions(options)
	var mu sync.Mutex
	var timer *time.Timer
	var stopped bool
	var generation uint64
	save := func(expectedGeneration uint64) {
		// Native reads dispatch to the main thread, where shutdown takes mu.
		rect := window.Bounds()
		maximised := window.IsMaximised()
		mu.Lock()
		defer mu.Unlock()
		if stopped || generation != expectedGeneration {
			return
		}
		if err := settings.SetWindow(shell.WindowBounds{
			X: rect.X, Y: rect.Y, Width: rect.Width, Height: rect.Height, Maximised: maximised,
		}); err != nil {
			log.Warn("save window bounds", "err", err)
		}
	}
	changed := func(*application.WindowEvent) {
		mu.Lock()
		defer mu.Unlock()
		if stopped {
			return
		}
		if timer != nil {
			timer.Stop()
		}
		generation++
		expectedGeneration := generation
		timer = time.AfterFunc(windowSaveDebounce, func() { save(expectedGeneration) })
	}
	window.OnWindowEvent(events.Common.WindowDidMove, changed)
	window.OnWindowEvent(events.Common.WindowDidResize, changed)
	app.OnShutdown(func() {
		mu.Lock()
		defer mu.Unlock()
		stopped = true
		if timer != nil {
			timer.Stop()
		}
	})
	return window
}

func focusMainWindow() {
	if window, ok := application.Get().Window.GetByName("main"); ok {
		window.UnMinimise()
		window.Show()
		window.Focus()
	}
}
