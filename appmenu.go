package main

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/shell"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	themeLight = "light"
	themeDark  = "dark"
	docsURL    = "https://get-bistro.com/docs"
	issuesURL  = "https://github.com/Thourum/Bistro/issues/new"
)

type appMenuDeps struct {
	app        *application.App
	settings   *shell.SettingsStore
	set        *services.Set
	updates    *shell.UpdateChecks
	show       func()
	ctx        context.Context
	log        *slog.Logger
	isDevBuild bool
}

type appMenu struct {
	menu        *application.Menu
	actionItems map[string]*application.MenuItem
	themeItems  map[string]*application.MenuItem
	densityItem *application.MenuItem
	deps        appMenuDeps
	sequenceMu  sync.Mutex
	sequence    int
	isStarted   atomic.Bool
	invoke      func(func())
}

func buildAppMenu(deps appMenuDeps) *appMenu {
	m := &appMenu{
		menu: application.NewMenu(), deps: deps, invoke: application.InvokeAsync,
		actionItems: make(map[string]*application.MenuItem),
		themeItems:  make(map[string]*application.MenuItem),
	}
	if runtime.GOOS == "darwin" {
		m.buildBistroMenu()
	}
	m.buildFileMenu()
	m.menu.AddRole(application.EditMenu)
	m.buildViewMenu()
	m.menu.AddRole(application.WindowMenu)
	m.buildHelpMenu()
	deps.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		m.isStarted.Store(true)
		m.syncAppearance()
	})
	return m
}

func (m *appMenu) addAction(menu *application.Menu, label, actionID string) {
	item := menu.Add(label)
	isEnabled := false
	item.SetEnabled(isEnabled)
	m.actionItems[actionID] = item
	item.OnClick(func(*application.Context) {
		m.deps.show()
		// Keep sequence assignment and emission in the same order for concurrent clicks.
		m.sequenceMu.Lock()
		defer m.sequenceMu.Unlock()
		m.sequence++
		m.deps.app.Event.Emit(services.EventMenuAction, services.MenuAction{
			ActionID: actionID, Sequence: m.sequence,
		})
	})
}

func (m *appMenu) addUpdateCheck(menu *application.Menu) {
	menu.Add("Check for Updates…").SetHidden(m.deps.isDevBuild).OnClick(func(*application.Context) {
		m.deps.updates.CheckNow()
	})
}

func (m *appMenu) buildBistroMenu() {
	menu := m.menu.AddSubmenu("Bistro")
	menu.AddRole(application.About)
	m.addAction(menu, "Settings…", "global.settings")
	m.addUpdateCheck(menu)
	menu.AddSeparator()
	menu.AddRole(application.ServicesMenu)
	menu.AddSeparator()
	for _, role := range []application.Role{application.Hide, application.HideOthers, application.UnHide} {
		menu.AddRole(role)
	}
	menu.AddSeparator()
	menu.AddRole(application.Quit)
}

func (m *appMenu) buildFileMenu() {
	menu := m.menu.AddSubmenu("File")
	menu.Add("Sync Now").OnClick(func(*application.Context) {
		go func() {
			if _, err := m.deps.set.Sync.Force(m.deps.ctx, services.ForceSyncRequest{}); err != nil {
				m.deps.log.Warn("menu sync failed", "err", err)
			}
		}()
	})
	for _, action := range []struct{ label, id string }{
		{label: "Inbox", id: "global.inbox"},
		{label: "Search", id: "global.search"},
		{label: "Command Palette", id: "global.command"},
	} {
		m.addAction(menu, action.label, action.id)
	}
	if runtime.GOOS != "darwin" {
		m.addAction(menu, "Settings…", "global.settings")
		m.addUpdateCheck(menu)
		menu.AddSeparator()
		menu.AddRole(application.Quit)
	}
}

func (m *appMenu) buildViewMenu() {
	menu := m.menu.AddSubmenu("View")
	themes := menu.AddSubmenu("Theme")
	appearance := m.deps.settings.Get().Appearance
	for _, theme := range []struct{ label, value string }{
		{label: "Light", value: themeLight},
		{label: "Dark", value: themeDark},
	} {
		isChecked := appearance.Theme == theme.value
		item := themes.AddRadio(theme.label, isChecked)
		m.themeItems[theme.value] = item
		item.OnClick(func(*application.Context) {
			m.updateAppearance(func(s *shell.Settings) { s.Appearance.Theme = theme.value })
		})
	}
	m.densityItem = menu.AddCheckbox("Compact Density", appearance.HighDensity)
	m.densityItem.OnClick(func(*application.Context) {
		m.updateAppearance(func(s *shell.Settings) { s.Appearance.HighDensity = !s.Appearance.HighDensity })
	})
	menu.AddSeparator()
	for _, role := range []application.Role{
		application.Reload, application.ResetZoom, application.ZoomIn,
		application.ZoomOut, application.ToggleFullscreen,
	} {
		menu.AddRole(role)
	}
}

func (m *appMenu) updateAppearance(mutate func(*shell.Settings)) {
	if _, err := m.deps.settings.Update(mutate); err != nil {
		m.deps.log.Warn("menu appearance update failed", "err", err)
		// Native clicks change checks before persistence; restore them after a failed write.
		m.syncAppearance()
	}
}

func (m *appMenu) buildHelpMenu() {
	menu := m.menu.AddSubmenu("Help")
	m.addAction(menu, "Keyboard Shortcuts", "global.help")
	for _, link := range []struct{ label, url string }{
		{label: "Documentation", url: docsURL},
		{label: "Report an Issue", url: issuesURL},
	} {
		menu.Add(link.label).OnClick(func(*application.Context) {
			go func() {
				if err := m.deps.set.Shell.OpenExternal(m.deps.ctx, link.url); err != nil {
					m.deps.log.Warn("menu external link failed", "err", err)
				}
			}()
		})
	}
	menu.Add("Open Logs").OnClick(func(*application.Context) {
		go func() {
			if err := m.deps.set.Shell.OpenLogFolder(m.deps.ctx); err != nil {
				m.deps.log.Warn("menu open logs failed", "err", err)
			}
		}()
	})
	if runtime.GOOS != "darwin" {
		menu.AddSeparator()
		menu.AddRole(application.About)
	}
}

// Item setters update attached native items; rebuilding the root detaches their impls on Windows.
func (m *appMenu) setEnabledActions(enabled []string) {
	allowed := make(map[string]bool, len(enabled))
	for _, id := range enabled {
		allowed[id] = true
	}
	m.invoke(func() {
		for id, item := range m.actionItems {
			item.SetEnabled(allowed[id])
		}
	})
}

func (m *appMenu) syncAppearance() {
	// Wails cannot dispatch before Run creates the platform app; startup reconciles the latest snapshot.
	if !m.isStarted.Load() {
		return
	}
	m.invoke(func() {
		// Reordered callbacks must reconcile current settings, not a captured older commit.
		settings := m.deps.settings.Get()
		for theme, item := range m.themeItems {
			item.SetChecked(settings.Appearance.Theme == theme)
		}
		m.densityItem.SetChecked(settings.Appearance.HighDensity)
	})
}
