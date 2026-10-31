package shell

import (
	"context"
	_ "embed"
	"log/slog"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const updateCheckInterval = 6 * time.Hour

//go:embed updater.css
var updaterCSS string

// UpdateChecks owns both the initial and periodic checks so shutdown can drain
// them off the native thread before Wails tears down its windows.
type UpdateChecks struct {
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	manual     sync.WaitGroup
	app        *application.App
	ctx        context.Context
	log        *slog.Logger
	isStopping bool
}

// Stop cancels checks and waits for their UI work. Call only off the native thread.
func (u *UpdateChecks) Stop() {
	u.mu.Lock()
	u.isStopping = true
	cancel, done := u.cancel, u.done
	u.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	u.manual.Wait()
}

// CheckNow opens Wails' updater window for a user-requested check.
// It returns false when updates are disabled or shutdown has begun.
func (u *UpdateChecks) CheckNow() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.app == nil || u.isStopping || u.ctx.Err() != nil {
		return false
	}
	app, ctx, log := u.app, u.ctx, u.log
	u.manual.Add(1)
	go func() {
		defer u.manual.Done()
		if err := app.Updater.CheckAndInstall(ctx); err != nil && ctx.Err() == nil {
			log.Warn("manual update check failed", "err", err)
		}
	}()
	return true
}

// Start checks GitHub releases at launch and every six hours.
// Dev and untagged builds never update.
func (u *UpdateChecks) Start(ctx context.Context, app *application.App, version string, log *slog.Logger) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if version == "" || version == "dev" {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	provider, err := github.New(github.Config{
		Repository: "Thourum/Bistro", Prerelease: true, ChecksumAsset: "SHA256SUMS",
	})
	if err != nil {
		return err
	}
	if err := app.Updater.Init(updater.Config{
		CurrentVersion: version,
		Providers:      []updater.Provider{provider},
		Window:         &updater.BuiltinWindow{CSS: updaterCSS},
	}); err != nil {
		return err
	}
	ctx, u.cancel = context.WithCancel(ctx)
	u.app, u.ctx, u.log = app, ctx, log
	u.done = make(chan struct{})
	go func() {
		defer close(u.done)
		ticker := time.NewTicker(updateCheckInterval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			state := app.Updater.State()
			if state == updater.StateReady || state == updater.StateChecking || state == updater.StateDownloading || state == updater.StateVerifying || state == updater.StateInstalling {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					continue
				}
			}
			if err := app.Updater.CheckAndInstall(ctx); err != nil && ctx.Err() == nil {
				log.Warn("update check failed", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}
