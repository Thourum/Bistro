package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/itterno/bistro/internal/shell"
)

func TestSmokeDataDirIgnoresPersistentOverride(t *testing.T) {
	persistentDir := filepath.Join(t.TempDir(), "persistent")
	t.Setenv("BISTRO_DATA_DIR", persistentDir)
	smokeDir, err := resolveDataDir(true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(smokeDir) })
	if smokeDir == persistentDir {
		t.Fatal("smoke used the persistent data directory")
	}
	if _, err := os.Stat(smokeDir); err != nil {
		t.Fatalf("smoke directory not created: %v", err)
	}
	if _, err := os.Stat(persistentDir); !os.IsNotExist(err) {
		t.Fatalf("smoke touched the persistent directory: %v", err)
	}
	normalDir, err := resolveDataDir(false)
	if err != nil || normalDir != persistentDir {
		t.Fatalf("normal launch did not honor override: %q, %v", normalDir, err)
	}
}

func TestSmokeSyncSettingsCannotEnableAutoSync(t *testing.T) {
	requested := shell.SyncSettings{AutoSync: true, SyncInterval: 60, Notifications: true}
	for _, enabled := range []bool{false, true} {
		requested.AutoSync = enabled
		got := effectiveSyncSettings(requested, true)
		if got.AutoSync || got.SyncInterval != requested.SyncInterval || got.Notifications != requested.Notifications {
			t.Fatalf("smoke settings = %+v", got)
		}
		if got := effectiveSyncSettings(requested, false); got != requested {
			t.Fatalf("normal settings altered: %+v", got)
		}
	}
}
