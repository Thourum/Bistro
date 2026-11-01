package tray

import (
	"embed"

	"github.com/itterno/bistro/internal/services"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed icons/*.png
var iconFiles embed.FS

// Actions contains the shell operations triggered by tray menu items.
type Actions struct {
	Show, SyncNow, CheckUpdates, OpenLogs, Quit func()
}

// Tray applies the platform-independent view through Wails' native tray API.
type Tray struct {
	tray    *application.SystemTray
	actions Actions
	opts    ViewOptions
}

// New must run before app.Run so Wails binds its default right-click menu.
func New(app *application.App, actions Actions, opts ViewOptions, initial services.SyncStatus) *Tray {
	t := &Tray{tray: app.SystemTray.New(), actions: actions, opts: opts}
	t.tray.OnClick(actions.Show)
	t.apply(BuildView(initial, opts))
	return t
}

// Update is safe for engine goroutines; native UI work runs on the main thread.
func (t *Tray) Update(status services.SyncStatus) {
	view := BuildView(status, t.opts)
	application.InvokeAsync(func() { t.apply(view) })
}

func (t *Tray) apply(view View) {
	// BuildView only selects embedded icons, so a missing asset is a build error.
	icon, err := iconFiles.ReadFile("icons/" + string(view.Icon) + ".png")
	if err != nil {
		panic(err)
	}
	t.tray.SetIcon(icon)
	t.tray.SetTooltip(view.Tooltip)
	menu := application.NewMenu()
	for _, item := range view.Items {
		if item.ID == ItemSeparator {
			menu.AddSeparator()
			continue
		}
		entry := menu.Add(item.Label).SetEnabled(item.Enabled)
		if handler := t.handler(item.ID); handler != nil {
			entry.OnClick(func(*application.Context) { handler() })
		}
	}
	t.tray.SetMenu(menu)
}

func (t *Tray) handler(id ItemID) func() {
	return map[ItemID]func(){
		ItemShow: t.actions.Show, ItemSyncNow: t.actions.SyncNow,
		ItemCheckUpdates: t.actions.CheckUpdates, ItemOpenLogs: t.actions.OpenLogs,
		ItemQuit: t.actions.Quit,
	}[id]
}
