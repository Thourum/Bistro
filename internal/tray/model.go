// Package tray turns engine sync status into a cross-platform tray view.
package tray

import (
	"fmt"
	"time"

	"github.com/itterno/bistro/internal/services"
)

// Icon identifies the platform-independent tray artwork.
type Icon string

const (
	IconIdle      Icon = "idle"
	IconSyncing   Icon = "syncing"
	IconAttention Icon = "attention"
	IconSignedOut Icon = "signed_out"
)

// ItemID identifies a tray menu entry or separator.
type ItemID string

const (
	ItemStatus       ItemID = "status"
	ItemSeparator    ItemID = "separator"
	ItemShow         ItemID = "show"
	ItemSyncNow      ItemID = "sync_now"
	ItemCheckUpdates ItemID = "check_updates"
	ItemOpenLogs     ItemID = "open_logs"
	ItemQuit         ItemID = "quit"
)

// Item describes one tray menu entry without platform callbacks.
type Item struct {
	ID      ItemID
	Label   string
	Enabled bool
}

// View is the complete platform-independent tray presentation.
type View struct {
	Icon       Icon
	Tooltip    string
	StatusLine string
	Items      []Item
}

// ViewOptions selects build-specific availability and the display timezone.
type ViewOptions struct {
	IsDevBuild bool
	Location   *time.Location
}

const (
	tooltipPrefix = "Bistro — "
	clockLayout   = "15:04"
)

// BuildView is pure so every platform shares one tested tray behaviour.
func BuildView(status services.SyncStatus, opts ViewOptions) View {
	upToDate := withClock("Up to date · synced ", status.LastSyncedAt, clockOptions{Location: opts.Location, Fallback: "Up to date"})
	states := map[string]struct {
		icon Icon
		line string
	}{
		services.SyncStateSignedOut:   {icon: IconSignedOut, line: "Signed out"},
		services.SyncStateSyncing:     {icon: IconSyncing, line: "Syncing…"},
		services.SyncStateUpToDate:    {icon: IconIdle, line: upToDate},
		services.SyncStatePaused:      {icon: IconIdle, line: "Paused (auto-sync off)"},
		services.SyncStateRateLimited: {icon: IconAttention, line: withClock("Rate limited · retry after ", status.ResumeAt, clockOptions{Location: opts.Location, Fallback: "Rate limited"})},
		services.SyncStateError:       {icon: IconAttention, line: fmt.Sprintf("Sync error (%s)", status.ErrorCode)},
	}
	presentation, ok := states[status.State]
	if !ok {
		presentation = states[services.SyncStateUpToDate]
	}
	unsyncableStates := map[string]bool{
		services.SyncStateSignedOut: true,
		services.SyncStateSyncing:   true,
	}
	return View{
		Icon: presentation.icon, Tooltip: tooltipPrefix + presentation.line, StatusLine: presentation.line,
		Items: []Item{
			{ID: ItemStatus, Label: presentation.line},
			{ID: ItemSeparator},
			{ID: ItemShow, Label: "Show Bistro", Enabled: true},
			{ID: ItemSyncNow, Label: "Sync Now", Enabled: !unsyncableStates[status.State]},
			{ID: ItemCheckUpdates, Label: "Check for Updates…", Enabled: !opts.IsDevBuild},
			{ID: ItemOpenLogs, Label: "Open Logs", Enabled: true},
			{ID: ItemSeparator},
			{ID: ItemQuit, Label: "Quit Bistro", Enabled: true},
		},
	}
}

type clockOptions struct {
	Location *time.Location
	Fallback string
}

// withClock falls back when the engine has no usable timestamp yet.
func withClock(prefix, timestamp string, opts clockOptions) string {
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return opts.Fallback
	}
	location := opts.Location
	if location == nil {
		location = time.Local
	}
	return prefix + parsed.In(location).Format(clockLayout)
}
