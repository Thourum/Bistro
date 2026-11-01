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
	mu              sync.Mutex
	cancel          context.CancelFunc
	done            chan struct{}
	manual          sync.WaitGroup
	checkAndInstall func(context.Context) error
	state           func() updater.State
	isChecking      bool
	ctx             context.Context
	log             *slog.Logger
	isStopping      bool
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
// Requests during an active check or staged update are accepted but ignored.
func (u *UpdateChecks) CheckNow() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.checkAndInstall == nil || u.isStopping || u.ctx.Err() != nil {
		return false
	}
	if !u.beginCheck() {
		u.log.Info("manual update check ignored: update in progress")
		return true
	}
	ctx, log := u.ctx, u.log
	u.manual.Add(1)
	go func() {
		defer u.manual.Done()
		if err := u.runCheck(ctx); err != nil && ctx.Err() == nil {
			log.Warn("manual update check failed", "err", err)
		}
	}()
	return true
}

// Caller holds mu: reserve before launching work, as Wails' state changes later.
// CheckAndInstall replaces its previous session (updater/updater.go:375-388).
func (u *UpdateChecks) beginCheck() bool {
	if u.isChecking {
		return false
	}
	switch u.state() {
	case updater.StateChecking, updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
		return false
	}
	u.isChecking = true
	return true
}

func (u *UpdateChecks) runCheck(ctx context.Context) error {
	defer func() {
		u.mu.Lock()
		u.isChecking = false
		u.mu.Unlock()
	}()
	return u.checkAndInstall(ctx)
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
	u.checkAndInstall, u.state = app.Updater.CheckAndInstall, app.Updater.State
	u.ctx, u.log = ctx, log
	u.done = make(chan struct{})
	go func() {
		defer close(u.done)
		ticker := time.NewTicker(updateCheckInterval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			u.mu.Lock()
			shouldCheck := !u.isStopping && ctx.Err() == nil && u.beginCheck()
			u.mu.Unlock()
			if shouldCheck {
				if err := u.runCheck(ctx); err != nil && ctx.Err() == nil {
					log.Warn("update check failed", "err", err)
				}
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
