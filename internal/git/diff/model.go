package diff

import "time"

// Diff status — mirrors the diff_status column in pr_diffs. Rows
// written before the per-PR budget was removed may still hold "too_large".
const (
	// StatusOK means the full patch is stored.
	StatusOK = "ok"
	// StatusTruncated means the per-file budget was exceeded; the patch is empty.
	StatusTruncated = "truncated"
)

// DefaultMaxFilePatchBytes caps one file's stored patch (safe for all SQLite page sizes).
const DefaultMaxFilePatchBytes int64 = 512 * 1024

// Options controls budget limits for Generate. A zero value means
// "no limit"; the sync path should always pass DefaultOptions().
type Options struct {
	MaxFilePatchBytes int64 // 0 = unlimited
}

// DefaultOptions returns production-safe budget defaults.
func DefaultOptions() Options {
	return Options{MaxFilePatchBytes: DefaultMaxFilePatchBytes}
}

// File represents a single file's diff
type File struct {
	Path         string // File path, e.g., "cmd/main.go"
	Status       string // "added", "modified", "deleted", "renamed", or "binary"
	DiffStatus   string // "ok" or "truncated"
	OldPath      string // Previous path for renames/deletes
	Binary       bool   // Whether the file patch is binary
	Truncated    bool   // Whether patch content was truncated
	PatchBytes   int    // Byte count of rendered patch before any truncation
	HunkCount    int    // Count of unified diff hunks before any truncation
	Patch        string // Unified diff content
	PatchID      string // PatchID of the full patch (before budget truncation)
	AddedLines   int    // Count of + lines
	RemovedLines int    // Count of - lines
}

// Profile captures local diff generation metrics for profiling and
// regression checks. Durations cover only local work after clone/cache hit has
// supplied a repository path; clone/fetch timing is logged separately by sync.
type Profile struct {
	TotalDuration     time.Duration
	OpenRepository    time.Duration
	LoadCommits       time.Duration
	LoadTrees         time.Duration
	BuildPatch        time.Duration
	RenderFilePatches time.Duration
	FileCount         int
	PatchBytes        int64
	BinaryFileCount   int
	TruncatedFiles    int // files where the per-file budget was exceeded
}

// Result contains all changed files
type Result struct {
	Files   []*File // All file diffs
	Size    int64   // Total bytes
	Profile Profile // Local generation metrics
}
