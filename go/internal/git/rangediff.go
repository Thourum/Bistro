package git

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// shaPattern matches a valid 40-char lowercase hex SHA.
// Used to guard all arguments passed to exec.Command to prevent injection.
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// RangeDiffResult is the output of RunRangeDiff.
type RangeDiffResult struct {
	// Raw is the raw text output of `git range-diff`.
	// Empty when Degraded=true.
	Raw string
	// Degraded is true when range-diff was unavailable and a fallback was used.
	Degraded bool
	// FallbackReason explains why range-diff was unavailable.
	// Empty when Degraded=false.
	FallbackReason string
}

// RangeDiffer wraps `git range-diff` with a one-time capability probe.
// If system git is absent or older than 2.19, all calls return Degraded=true.
// Thread-safe after construction (all mutable state is set during NewRangeDiffer).
type RangeDiffer struct {
	log            *slog.Logger
	gitClient      *Client // optional; used for fallback go-git diff
	available      bool
	fallbackReason string
}

// NewRangeDiffer constructs a RangeDiffer and immediately probes for git availability.
// The probe result is cached; it is never re-run per diff call.
func NewRangeDiffer(log *slog.Logger, gitClient *Client) *RangeDiffer {
	if log == nil {
		log = slog.Default()
	}
	rd := &RangeDiffer{log: log, gitClient: gitClient}
	rd.probeGit()
	return rd
}

// Available reports whether system git >= 2.19 was found at construction time.
func (rd *RangeDiffer) Available() bool { return rd.available }

// FallbackReason returns the human-readable reason why range-diff is unavailable.
// Empty when Available() is true.
func (rd *RangeDiffer) FallbackReason() string { return rd.fallbackReason }

// parseVersionAndSetAvailable parses a "git version X.Y.Z" string and sets
// available/fallbackReason accordingly. Extracted from probeGit for testability.
func (rd *RangeDiffer) parseVersionAndSetAvailable(versionLine string) {
	parts := strings.Fields(strings.TrimSpace(versionLine))
	if len(parts) < 3 {
		rd.available = false
		rd.fallbackReason = fmt.Sprintf("unparseable git version output: %q", versionLine)
		rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
		return
	}

	ver := parts[2]
	segments := strings.SplitN(ver, ".", 3)
	if len(segments) < 2 {
		rd.available = false
		rd.fallbackReason = fmt.Sprintf("unparseable version %q", ver)
		rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
		return
	}

	major, errM := strconv.Atoi(segments[0])
	minor, errN := strconv.Atoi(strings.Split(segments[1], "-")[0])
	if errM != nil || errN != nil {
		rd.available = false
		rd.fallbackReason = fmt.Sprintf("could not parse version numbers from %q", ver)
		rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
		return
	}

	if major > 2 || (major == 2 && minor >= 19) {
		rd.available = true
		rd.log.Info("git range-diff available", "version", ver)
	} else {
		rd.available = false
		rd.fallbackReason = fmt.Sprintf("git %s < 2.19 (range-diff requires >= 2.19)", ver)
		rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
	}
}

// probeGit runs `git --version` with a 3-second timeout and caches the result.
func (rd *RangeDiffer) probeGit() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "git", "--version").Output()
	if err != nil {
		rd.available = false
		rd.fallbackReason = fmt.Sprintf("git not found or probe failed: %v", err)
		rd.log.Info("git range-diff unavailable", "reason", rd.fallbackReason)
		return
	}

	rd.parseVersionAndSetAvailable(strings.TrimSpace(string(out)))
}

// RunRangeDiff executes `git range-diff <baseStart>..<baseEnd> <headStart>..<headEnd>`
// in repoPath with a 30-second timeout.
//
// SHA injection guard: all four SHA arguments must be exactly 40 lowercase hex
// characters; any other value returns an error before exec is called.
//
// repoPath validation: an empty repoPath is an explicit error (never falls back
// to process cwd). A non-existent path also returns an error. Callers that do
// not have a local clone should check Available() and handle gracefully before
// calling RunRangeDiff — or let the error propagate to their own fallback logic.
//
// When range-diff capability is unavailable (Available()==false) the call returns
// a degraded result with no error — callers should provide their own fallback.
//
// A non-nil error is returned only for hard failures (invalid input, context
// cancelled, repoPath validation failure).
func (rd *RangeDiffer) RunRangeDiff(
	ctx context.Context,
	repoPath, baseStart, baseEnd, headStart, headEnd string,
) (RangeDiffResult, error) {
	if !rd.available {
		return rd.fallback(), nil
	}

	// repoPath validation: must be non-empty and must exist.
	if repoPath == "" {
		return RangeDiffResult{}, errors.New("range-diff: repoPath is empty (would fall back to process cwd)")
	}
	if _, err := os.Stat(repoPath); err != nil {
		return RangeDiffResult{}, fmt.Errorf("range-diff: repoPath %q is not accessible: %w", repoPath, err)
	}

	// Injection guard: validate all SHA arguments before handing them to exec.
	for _, arg := range []string{baseStart, baseEnd, headStart, headEnd} {
		if !shaPattern.MatchString(arg) {
			return RangeDiffResult{}, fmt.Errorf(
				"range-diff: SHA argument %q is not 40-char lowercase hex", arg)
		}
	}

	rangeArg1 := baseStart + ".." + baseEnd
	rangeArg2 := headStart + ".." + headEnd

	// Bounded timeout so a stuck git process cannot block a handler forever.
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "git", "range-diff", rangeArg1, rangeArg2)
	cmd.Dir = repoPath

	out, err := cmd.Output()
	if err != nil {
		// Non-zero exit is treated as degraded (e.g. incompatible history);
		// context cancellation is a hard error.
		if runCtx.Err() != nil {
			return RangeDiffResult{}, fmt.Errorf("range-diff timed out or cancelled: %w", runCtx.Err())
		}
		reason := fmt.Sprintf("git range-diff exited non-zero: %v", err)
		rd.log.Warn("range-diff non-zero exit, degrading", "error", err)
		return RangeDiffResult{Degraded: true, FallbackReason: reason}, nil
	}

	return RangeDiffResult{Raw: string(out)}, nil
}

// fallback returns a degraded RangeDiffResult with the reason captured at probe time.
func (rd *RangeDiffer) fallback() RangeDiffResult {
	reason := rd.fallbackReason
	if reason == "" {
		reason = "range-diff unavailable"
	}
	return RangeDiffResult{Degraded: true, FallbackReason: reason}
}
