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

// Diff status values — mirrors the diff_status column in pr_diffs. Rows
// written before the per-PR budget was removed may still hold "too_large".
const (
	DiffStatusOK        = "ok"        // full patch stored
	DiffStatusTruncated = "truncated" // per-file budget exceeded; patch empty
)

// DefaultMaxFilePatchBytes caps one file's stored patch (safe for all SQLite page sizes).
const DefaultMaxFilePatchBytes int64 = 512 * 1024

// DiffOptions controls budget limits for GenerateDiff. A zero value means
// "no limit"; the sync path should always pass DefaultDiffOptions().
type DiffOptions struct {
	MaxFilePatchBytes int64 // 0 = unlimited
}

// DefaultDiffOptions returns the production-safe budget defaults.
func DefaultDiffOptions() DiffOptions {
	return DiffOptions{MaxFilePatchBytes: DefaultMaxFilePatchBytes}
}

// DiffFile represents a single file's diff
type DiffFile struct {
	Path         string // File path, e.g., "cmd/main.go"
	Status       string // "added", "modified", "deleted", "renamed", or "binary"
	DiffStatus   string // "ok" or "truncated"
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
}

// DiffResult contains all changed files
type DiffResult struct {
	Files   []*DiffFile // All file diffs
	Size    int64       // Total bytes
	Profile DiffProfile // Local generation metrics
}
