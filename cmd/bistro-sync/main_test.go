package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupLoggingRestrictsNewAndExistingLogFiles(t *testing.T) {
	for _, isExisting := range []bool{false, true} {
		t.Run(map[bool]string{false: "new", true: "existing"}[isExisting], func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "sync.log")
			if isExisting {
				if err := os.WriteFile(logPath, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			closer, err := setupLogging("debug", logPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = closer.Close() })
			info, err := os.Stat(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("log permissions = %o, want 600", info.Mode().Perm())
			}
		})
	}
}
