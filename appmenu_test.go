package main

import (
	"testing"

	"github.com/itterno/bistro/internal/shell"
	"github.com/wailsapp/wails/v3/pkg/application"
)

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
	menu.syncAppearance(settings)
	if !menu.themeItems[themeLight].Checked() || menu.themeItems[themeDark].Checked() || menu.densityItem.Checked() {
		t.Fatal("appearance touched native items before startup")
	}
}
