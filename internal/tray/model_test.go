package tray

import (
	"reflect"
	"testing"
	"time"

	"github.com/itterno/bistro/internal/services"
)

func TestBuildView(t *testing.T) {
	const at = "2026-10-31T18:04:00Z"
	const invalidTimestamp = "not-a-timestamp"
	const timezoneOffset = 2 * 60 * 60
	for _, test := range []struct {
		name       string
		status     services.SyncStatus
		isDevBuild bool
		location   *time.Location
		wantLine   string
		wantIcon   Icon
		canSync    bool
		canUpdate  bool
	}{
		{name: "signed out", status: services.SyncStatus{State: services.SyncStateSignedOut}, wantLine: "Signed out", wantIcon: IconSignedOut, canUpdate: true},
		{name: "syncing", status: services.SyncStatus{State: services.SyncStateSyncing}, wantLine: "Syncing…", wantIcon: IconSyncing, canUpdate: true},
		{name: "up to date", status: services.SyncStatus{State: services.SyncStateUpToDate, LastSyncedAt: at}, location: time.UTC, wantLine: "Up to date · synced 18:04", wantIcon: IconIdle, canSync: true, canUpdate: true},
		{name: "never synced", status: services.SyncStatus{State: services.SyncStateUpToDate}, wantLine: "Up to date", wantIcon: IconIdle, canSync: true, canUpdate: true},
		{name: "invalid sync timestamp", status: services.SyncStatus{State: services.SyncStateUpToDate, LastSyncedAt: invalidTimestamp}, wantLine: "Up to date", wantIcon: IconIdle, canSync: true, canUpdate: true},
		{name: "paused", status: services.SyncStatus{State: services.SyncStatePaused}, wantLine: "Paused (auto-sync off)", wantIcon: IconIdle, canSync: true, canUpdate: true},
		{name: "rate limited", status: services.SyncStatus{State: services.SyncStateRateLimited, ResumeAt: at}, location: time.UTC, wantLine: "Rate limited · retry after 18:04", wantIcon: IconAttention, canSync: true, canUpdate: true},
		{name: "missing resume timestamp", status: services.SyncStatus{State: services.SyncStateRateLimited}, wantLine: "Rate limited", wantIcon: IconAttention, canSync: true, canUpdate: true},
		{name: "invalid resume timestamp", status: services.SyncStatus{State: services.SyncStateRateLimited, ResumeAt: invalidTimestamp}, wantLine: "Rate limited", wantIcon: IconAttention, canSync: true, canUpdate: true},
		{name: "error", status: services.SyncStatus{State: services.SyncStateError, ErrorCode: "SYNC_FAILED"}, wantLine: "Sync error (SYNC_FAILED)", wantIcon: IconAttention, canSync: true, canUpdate: true},
		{name: "dev build", status: services.SyncStatus{State: services.SyncStateUpToDate}, isDevBuild: true, wantLine: "Up to date", wantIcon: IconIdle, canSync: true},
		{name: "unknown state", status: services.SyncStatus{State: "unknown", LastSyncedAt: at}, location: time.UTC, wantLine: "Up to date · synced 18:04", wantIcon: IconIdle, canSync: true, canUpdate: true},
		{name: "explicit timezone", status: services.SyncStatus{State: services.SyncStateRateLimited, ResumeAt: at}, location: time.FixedZone("test", timezoneOffset), wantLine: "Rate limited · retry after 20:04", wantIcon: IconAttention, canSync: true, canUpdate: true},
		{name: "local timezone default", status: services.SyncStatus{State: services.SyncStateUpToDate, LastSyncedAt: at}, wantLine: "Up to date · synced " + time.Date(2026, time.October, 31, 18, 4, 0, 0, time.UTC).In(time.Local).Format("15:04"), wantIcon: IconIdle, canSync: true, canUpdate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := BuildView(test.status, ViewOptions{IsDevBuild: test.isDevBuild, Location: test.location})
			want := View{
				Icon: test.wantIcon, Tooltip: "Bistro — " + test.wantLine, StatusLine: test.wantLine,
				Items: []Item{
					{ID: ItemStatus, Label: test.wantLine},
					{ID: ItemSeparator},
					{ID: ItemShow, Label: "Show Bistro", Enabled: true},
					{ID: ItemSyncNow, Label: "Sync Now", Enabled: test.canSync},
					{ID: ItemCheckUpdates, Label: "Check for Updates…", Enabled: test.canUpdate},
					{ID: ItemOpenLogs, Label: "Open Logs", Enabled: true},
					{ID: ItemSeparator},
					{ID: ItemQuit, Label: "Quit Bistro", Enabled: true},
				},
			}
			if !reflect.DeepEqual(view, want) {
				t.Fatalf("BuildView() = %+v, want %+v", view, want)
			}
		})
	}
}
