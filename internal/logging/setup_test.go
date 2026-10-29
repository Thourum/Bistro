package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupValidatesLevelBeforeCreatingFile(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	filename := filepath.Join(t.TempDir(), "logs", "bistro.log")
	if _, _, err := Setup("verbose", filename); err == nil {
		t.Fatal("invalid log level accepted")
	}
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		t.Fatalf("invalid configuration created log file: %v", err)
	}
	for _, test := range []struct {
		level   string
		minimum slog.Level
	}{
		{"", slog.LevelInfo}, {"debug", slog.LevelDebug}, {"info", slog.LevelInfo},
		{"warn", slog.LevelWarn}, {"error", slog.LevelError},
	} {
		logger, closeLog, err := Setup(test.level, filename)
		if err != nil {
			t.Fatal(err)
		}
		if !logger.Enabled(context.Background(), test.minimum) || logger.Enabled(context.Background(), test.minimum-1) {
			t.Errorf("level %q has wrong threshold", test.level)
		}
		closeLog()
	}
	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("log permissions = %o", info.Mode().Perm())
	}
}
