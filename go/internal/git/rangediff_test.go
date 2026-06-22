package git

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// newSilentLogger returns a logger that discards all output, reducing test noise.
func newSilentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestRangeDiffer_ProbeAvailable(t *testing.T) {
	// On any modern developer machine git >= 2.19 should be present.
	// This is a best-effort check; the test is skipped if git is not in PATH.
	rd := NewRangeDiffer(newSilentLogger(), nil)
	if rd == nil {
		t.Fatal("NewRangeDiffer returned nil")
	}
	// Probe must have run without panic; Available/FallbackReason are consistent.
	if rd.Available() && rd.FallbackReason() != "" {
		t.Errorf("Available=true but FallbackReason=%q (should be empty)", rd.FallbackReason())
	}
	if !rd.Available() && rd.FallbackReason() == "" {
		t.Error("Available=false but FallbackReason is empty (should explain why)")
	}
}

func TestRangeDiffer_ProbeTooOld(t *testing.T) {
	// Simulate an old git by injecting a fake version string.
	rd := &RangeDiffer{log: newSilentLogger()}
	rd.parseVersionAndSetAvailable("git version 2.18.0")
	if rd.Available() {
		t.Error("version 2.18.0 should be unavailable (< 2.19)")
	}
	if rd.FallbackReason() == "" {
		t.Error("FallbackReason should be non-empty when too old")
	}
}

func TestRangeDiffer_ProbeExact219(t *testing.T) {
	rd := &RangeDiffer{log: newSilentLogger()}
	rd.parseVersionAndSetAvailable("git version 2.19.0")
	if !rd.Available() {
		t.Errorf("version 2.19.0 should be available, reason: %q", rd.FallbackReason())
	}
}

func TestRangeDiffer_ProbeNewMajor(t *testing.T) {
	rd := &RangeDiffer{log: newSilentLogger()}
	rd.parseVersionAndSetAvailable("git version 3.0.0")
	if !rd.Available() {
		t.Errorf("version 3.0.0 should be available, reason: %q", rd.FallbackReason())
	}
}

func TestRangeDiffer_ProbeUnparseable(t *testing.T) {
	rd := &RangeDiffer{log: newSilentLogger()}
	rd.parseVersionAndSetAvailable("not a git version at all")
	if rd.Available() {
		t.Error("unparseable output should result in unavailable")
	}
	if rd.FallbackReason() == "" {
		t.Error("FallbackReason should be set for unparseable output")
	}
}

func TestRangeDiffer_InjectionGuard_SHA(t *testing.T) {
	rd := &RangeDiffer{log: newSilentLogger(), available: true}

	badSHAs := []string{
		"",
		"abc",
		"00000000000000000000000000000000000000GG", // non-hex
		"0000000000000000000000000000000000000000000", // 41 chars
		"' ; rm -rf /",
	}

	valid := strings.Repeat("0", 40)
	ctx := context.Background()
	for _, bad := range badSHAs {
		_, err := rd.RunRangeDiff(ctx, "/tmp", bad, valid, valid, valid)
		if err == nil {
			t.Errorf("injection guard: expected error for SHA %q, got nil", bad)
		}
	}
}

func TestRangeDiffer_FallbackWhenUnavailable(t *testing.T) {
	rd := &RangeDiffer{
		log:            newSilentLogger(),
		available:      false,
		fallbackReason: "git not found",
	}
	valid := strings.Repeat("a", 40)
	result, err := rd.RunRangeDiff(context.Background(), "/tmp", valid, valid, valid, valid)
	if err != nil {
		t.Fatalf("fallback should not return error, got: %v", err)
	}
	if !result.Degraded {
		t.Error("fallback result should have Degraded=true")
	}
	if result.FallbackReason == "" {
		t.Error("fallback result should have non-empty FallbackReason")
	}
}

func TestRangeDiffer_EmptyRepoPath_Error(t *testing.T) {
	// Empty repoPath must return an explicit error when git is available.
	// This prevents cmd.Dir="" from silently running in the process cwd.
	rd := &RangeDiffer{log: newSilentLogger(), available: true}
	valid := strings.Repeat("a", 40)
	_, err := rd.RunRangeDiff(context.Background(), "", valid, valid, valid, valid)
	if err == nil {
		t.Error("empty repoPath should return error, got nil")
	}
}

func TestRangeDiffer_NonExistentRepoPath_Error(t *testing.T) {
	// Non-existent repoPath must return an explicit error.
	rd := &RangeDiffer{log: newSilentLogger(), available: true}
	valid := strings.Repeat("a", 40)
	_, err := rd.RunRangeDiff(context.Background(), "/tmp/bistro-nonexistent-repo-12345", valid, valid, valid, valid)
	if err == nil {
		t.Error("non-existent repoPath should return error, got nil")
	}
}
