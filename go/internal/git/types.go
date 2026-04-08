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

// DiffFile represents a single file's diff
type DiffFile struct {
	Path         string // File path, e.g., "cmd/main.go"
	Status       string // "added", "modified", "deleted"
	Patch        string // Unified diff content
	AddedLines   int    // Count of + lines
	RemovedLines int    // Count of - lines
}

// DiffResult contains all changed files
type DiffResult struct {
	Files []*DiffFile // All file diffs
	Size  int64       // Total bytes
}
