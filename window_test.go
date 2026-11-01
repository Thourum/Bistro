package main

import (
	"github.com/itterno/bistro/internal/shell"
	"testing"
)

// The smoke window must never start background sync with persisted preferences.
func TestSmokeWindowDisablesSyncWithoutChangingPreferences(t *testing.T) {
	preferences := shell.SyncSettings{AutoSync: true, SyncInterval: 60, Notifications: true}
	smoke := effectiveSyncSettings(preferences, true)
	if smoke.AutoSync || smoke.SyncInterval != preferences.SyncInterval || !smoke.Notifications {
		t.Fatalf("smoke settings = %+v", smoke)
	}
	if !preferences.AutoSync || effectiveSyncSettings(preferences, false) != preferences {
		t.Fatal("smoke changed normal launch preferences")
	}
}
