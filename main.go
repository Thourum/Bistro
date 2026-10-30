package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/itterno/bistro/internal/engine"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/shell"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

var version = "dev"

// embeddedGitHubClientID defaults to Bistro's own public OAuth app, whose
// secret lives on get-bistro.com. Override with GITHUB_CLIENT_ID or -ldflags -X.
// The client ID is public; the client secret must never be embedded.
var embeddedGitHubClientID = "Ov23li9pgeFw42MTBEvT"
var sentryDSN string

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cleanup := &appCleanup{}
	defer cleanup.close()
	flags := flag.NewFlagSet("bistro", flag.ContinueOnError)
	smoke := flags.Bool("smoke", false, "verify the native window and embedded frontend, then quit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	dataDir, err := resolveDataDir(*smoke)
	if err != nil {
		return err
	}
	if *smoke {
		cleanup.add(func() { _ = os.RemoveAll(dataDir) })
	}
	logDir := filepath.Join(dataDir, "logs")
	log, closeLog, err := logging.Setup(os.Getenv("BISTRO_LOG_LEVEL"), filepath.Join(logDir, "bistro.log"))
	if err != nil {
		return err
	}
	cleanup.add(closeLog)
	settings := shell.NewSettingsStore(filepath.Join(dataDir, "settings.json"), log)
	if err := settings.Load(); err != nil {
		return err
	}
	flushSentry, err := shell.InitSentry(sentryDSN, version, func() bool {
		return !*smoke && settings.Get().Privacy.TelemetryEnabled
	})
	if err != nil {
		log.Warn("sentry disabled", "err", err)
	}
	cleanup.add(flushSentry)
	reporter := shell.SentryReporter{Log: log}
	// Smoke must exercise this build, not focus an already-running Bistro.
	var single *application.SingleInstanceOptions
	if !*smoke {
		single = &application.SingleInstanceOptions{
			UniqueID:               "app.bistro.review",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { focusMainWindow() },
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cleanup.add(cancel)
	updates := &shell.UpdateChecks{}
	quit := &quitCoordinator{stop: func() { cancel(); updates.Stop() }}
	app := application.New(application.Options{
		ShouldQuit:     quit.shouldQuit,
		Name:           "Bistro",
		Assets:         application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		SingleInstance: single,
		PanicHandler: func(p *application.PanicDetails) {
			reporter.ReportPanic("wails", p.Error, []byte(p.FullStackTrace))
		},
		Mac: application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	quit.quit = app.Quit
	snapshot := settings.Get()
	syncSettings := effectiveSyncSettings(snapshot.Sync, *smoke)
	clientID, clientSecret := resolveGitHubOAuthCredentials()
	eng := engine.New(engine.Options{
		DBPath: filepath.Join(dataDir, "sync.db"), Log: log,
		AutoSync:           syncSettings.AutoSync,
		SyncInterval:       time.Duration(syncSettings.SyncInterval * float64(time.Second)),
		ReplayQueue:        !*smoke,
		GitHubClientID:     clientID,
		GitHubClientSecret: clientSecret,
	})
	eng.SetEmitter(services.EmitterFunc(func(name string, data any) { app.Event.Emit(name, data) }))
	eng.SetPanicReporter(reporter)
	if err := eng.Start(ctx); err != nil {
		return err
	}
	set := eng.Services()
	set.Shell = &services.Shell{
		Settings: settings, Platform: shell.NewPlatform(), EditorEnv: func() string { return os.Getenv("EDITOR") },
		Version: version, LogDir: logDir, Panics: reporter,
	}
	registerServices(app, set)
	tel := shell.NewTelemetry(dataDir, log)
	cleanup.add(tel.Close)
	cleanup.add(eng.Stop)
	applySettings(eng, tel, snapshot, *smoke, log)
	tel.TrackAppStarted(version)
	settings.OnChange(func(prev, next shell.Settings) {
		applySettings(eng, tel, next, *smoke, log)
		tel.TrackSettingsChanged(prev, next)
		app.Event.Emit(services.EventSettingsChanged, next)
	})
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		settings.Watch(ctx, 2*time.Second)
	}()
	cleanup.add(func() {
		cancel()
		<-watchDone
	})
	window := newMainWindow(app, settings, log)
	setMenu(app)
	smokeOK := armSmoke(app, window, smokeOptions{enabled: *smoke, dataDir: dataDir, log: log})
	// Window timers and smoke watchdog stop before their resources are closed.
	app.OnShutdown(cleanup.close)
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if !*smoke {
			if err := updates.Start(ctx, app, version, log); err != nil {
				log.Warn("updater disabled", "err", err)
			}
		}
	})
	if err := app.Run(); err != nil {
		return err
	}
	if *smoke && !smokeOK.Load() {
		return fmt.Errorf("smoke ended before window runtime was ready")
	}
	return nil
}

func resolveGitHubOAuthCredentials() (clientID, clientSecret string) {
	clientID = os.Getenv("GITHUB_CLIENT_ID")
	if clientID == "" {
		// A secret alone must not route Bistro's embedded app to direct exchange.
		return embeddedGitHubClientID, ""
	}
	return clientID, os.Getenv("GITHUB_CLIENT_SECRET")
}

func resolveDataDir(smoke bool) (string, error) {
	if smoke {
		return os.MkdirTemp("", "bistro-smoke-*")
	}
	dir := os.Getenv("BISTRO_DATA_DIR")
	if dir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(configDir, "Bistro")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func applySettings(eng *engine.App, tel *shell.Telemetry, s shell.Settings, smoke bool, log *slog.Logger) {
	syncSettings := effectiveSyncSettings(s.Sync, smoke)
	eng.ApplySyncSettings(syncSettings.AutoSync, time.Duration(syncSettings.SyncInterval*float64(time.Second)))
	if err := eng.ApplyClassificationSettings(services.ClassificationSettings{
		Mode: s.Classification.Mode, DismissalScope: s.Classification.DismissalScope,
		Provider: s.Classification.Provider, Model: s.Classification.Model, Endpoint: s.Classification.Endpoint,
	}); err != nil {
		log.Warn("classification settings rejected", "err", err)
	}
	tel.SetEnabled(!smoke && s.Privacy.TelemetryEnabled)
}

func registerServices(app *application.App, set *services.Set) {
	app.RegisterService(application.NewService(set.Shell))
	app.RegisterService(application.NewService(set.Expertise))
	app.RegisterService(application.NewService(set.Classification))
	app.RegisterService(application.NewService(set.Editor))
	app.RegisterService(application.NewService(set.RepoLocation))
	app.RegisterService(application.NewService(set.DevFolderScan))
	app.RegisterService(application.NewService(set.Comments))
	app.RegisterService(application.NewService(set.Dismiss))
	app.RegisterService(application.NewService(set.Threads))
	app.RegisterService(application.NewService(set.Drafts))
	app.RegisterService(application.NewService(set.Diff))
	app.RegisterService(application.NewService(set.Patchsets))
	app.RegisterService(application.NewService(set.Triage))
	app.RegisterService(application.NewService(set.Interdiff))
	app.RegisterService(application.NewService(set.Viewed))
	app.RegisterService(application.NewService(set.ReadState))
	app.RegisterService(application.NewService(set.Auth))
	app.RegisterService(application.NewService(set.PR))
	app.RegisterService(application.NewService(set.Sync))
	app.RegisterService(application.NewService(set.Repos))
	app.RegisterService(application.NewService(set.ReviewQueue))
}

func setMenu(app *application.App) {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	app.Menu.SetApplicationMenu(menu)
}

type smokeOptions struct {
	enabled bool
	dataDir string
	log     *slog.Logger
}

func effectiveSyncSettings(settings shell.SyncSettings, smoke bool) shell.SyncSettings {
	if smoke {
		settings.AutoSync = false
	}
	return settings
}

func armSmoke(app *application.App, window *application.WebviewWindow, options smokeOptions) *atomic.Bool {
	ok := &atomic.Bool{}
	if !options.enabled {
		return ok
	}
	timer := time.AfterFunc(60*time.Second, func() {
		options.log.Error("smoke timed out waiting for window runtime")
		if err := os.RemoveAll(options.dataDir); err != nil {
			options.log.Error("remove smoke data directory", "err", err)
		}
		os.Exit(2)
	})
	app.OnShutdown(func() { timer.Stop() })
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		ok.Store(true)
		timer.Stop()
		app.Quit()
	})
	return ok
}
