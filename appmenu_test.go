package main

import (
	"path/filepath"
	"testing"

	"github.com/itterno/bistro/internal/shell"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestAppMenuReconcilesLatestAppearanceInReorderedCallbacks(t *testing.T) {
	store := shell.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"), nil)
	var callbacks []func()
	isChecked := false
	menu := &appMenu{
		deps:   appMenuDeps{settings: store},
		invoke: func(callback func()) { callbacks = append(callbacks, callback) },
		themeItems: map[string]*application.MenuItem{
			themeLight: application.NewMenuItemRadio("Light", isChecked),
			themeDark:  application.NewMenuItemRadio("Dark", isChecked),
		},
		densityItem: application.NewMenuItemCheckbox("Compact Density", isChecked),
	}
	menu.isStarted.Store(true)
	menu.syncAppearance()
	if _, err := store.Update(func(s *shell.Settings) {
		s.Appearance.Theme = themeDark
		s.Appearance.HighDensity = true
	}); err != nil {
		t.Fatal(err)
	}
	menu.syncAppearance()
	// The newer commit's callback can run before the startup callback.
	for _, callback := range []func(){callbacks[1], callbacks[0]} {
		callback()
		if menu.themeItems[themeLight].Checked() || !menu.themeItems[themeDark].Checked() || !menu.densityItem.Checked() {
			t.Fatal("a queued callback restored stale appearance")
		}
	}
}

func TestAppMenuDefersAppearanceUntilNativeStartup(t *testing.T) {
	settings := shell.DefaultSettings()
	isLightChecked := settings.Appearance.Theme == themeLight
	isDarkChecked := settings.Appearance.Theme == themeDark
	menu := &appMenu{
		themeItems: map[string]*application.MenuItem{
			themeLight: application.NewMenuItemRadio("Light", isLightChecked),
			themeDark:  application.NewMenuItemRadio("Dark", isDarkChecked),
		},
		densityItem: application.NewMenuItemCheckbox("Compact Density", settings.Appearance.HighDensity),
	}
	settings.Appearance.Theme = themeDark
	settings.Appearance.HighDensity = true
	// No platform app exists yet. Dispatching here crashed the native smoke launch.
	menu.syncAppearance()
	if !menu.themeItems[themeLight].Checked() || menu.themeItems[themeDark].Checked() || menu.densityItem.Checked() {
		t.Fatal("appearance touched native items before startup")
	}
}
