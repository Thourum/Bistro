package git

import "time"

// RepoEntry represents a cached repository
type RepoEntry struct {
	URL       string    // GitHub repository URL
	LocalPath string    // Local filesystem path
	BaseSHA   string    // Base commit SHA
	HeadSHA   string    // Head commit SHA
	FetchedAt time.Time // When this was cached
}

// CloneResult tracks successful clone
type CloneResult struct {
	LocalPath string // Where repo is stored
	BaseSHA   string // Base commit SHA
	HeadSHA   string // Head commit SHA
}

// Diff status values — mirrors the diff_status column in pr_diffs.
const (
	DiffStatusOK        = "ok"        // full patch stored
	DiffStatusTruncated = "truncated" // per-file budget exceeded; patch empty
	DiffStatusTooLarge  = "too_large" // PR-level budget exhausted; patch empty
)

// Default budget limits (conservative: safe for all SQLite page sizes).
const (
	DefaultMaxFilePatchBytes  int64 = 512 * 1024      // 512 KB per file
	DefaultMaxTotalPatchBytes int64 = 4 * 1024 * 1024 // 4 MB per PR
)

// DiffOptions controls budget limits for GenerateDiff.
// Zero values mean "no limit" to allow callers that pre-date budget enforcement
// to keep working, but the sync path should always pass DefaultDiffOptions().
type DiffOptions struct {
	MaxFilePatchBytes  int64 // 0 = unlimited
	MaxTotalPatchBytes int64 // 0 = unlimited
}

// DefaultDiffOptions returns the production-safe budget defaults.
func DefaultDiffOptions() DiffOptions {
	return DiffOptions{
		MaxFilePatchBytes:  DefaultMaxFilePatchBytes,
		MaxTotalPatchBytes: DefaultMaxTotalPatchBytes,
	}
}

// DiffFile represents a single file's diff
type DiffFile struct {
	Path         string // File path, e.g., "cmd/main.go"
	Status       string // "added", "modified", "deleted", "renamed", or "binary"
	DiffStatus   string // "ok", "truncated", "too_large"
	OldPath      string // Previous path for renames/deletes
	Binary       bool   // Whether the file patch is binary
	Truncated    bool   // Whether the patch content was truncated
	PatchBytes   int    // Byte count of the rendered patch before any truncation
	HunkCount    int    // Count of unified diff hunks before any truncation
	Patch        string // Unified diff content
	AddedLines   int    // Count of + lines
	RemovedLines int    // Count of - lines
}

// DiffProfile captures local diff generation metrics for profiling and
// regression checks. Durations cover only local work after a clone/cache hit has
// supplied a repository path; clone/fetch timing is logged separately by sync.
type DiffProfile struct {
	TotalDuration     time.Duration
	OpenRepository    time.Duration
	LoadCommits       time.Duration
	LoadTrees         time.Duration
	BuildPatch        time.Duration
	RenderFilePatches time.Duration
	FileCount         int
	PatchBytes        int64
	BinaryFileCount   int
	TruncatedFiles    int // files where per-file budget was exceeded
	TooLargeFiles     int // files skipped because PR-level budget was exhausted
}

// DiffResult contains all changed files
type DiffResult struct {
	Files   []*DiffFile // All file diffs
	Size    int64       // Total bytes
	Profile DiffProfile // Local generation metrics
}
