package shell

import (
	"context"
	_ "embed"
	"log/slog"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const updateCheckInterval = 6 * time.Hour

//go:embed updater.css
var updaterCSS string

// StartUpdater checks GitHub releases at launch and every six hours.
// Dev and untagged builds never update.
func StartUpdater(ctx context.Context, app *application.App, version string, log *slog.Logger) error {
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
		CheckInterval:  updateCheckInterval,
		Window:         &updater.BuiltinWindow{CSS: updaterCSS},
	}); err != nil {
		return err
	}
	go func() {
		if err := app.Updater.CheckAndInstall(ctx); err != nil {
			log.Warn("update check failed", "err", err)
		}
	}()
	return nil
}
