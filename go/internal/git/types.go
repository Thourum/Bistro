package git

import "time"

// RepoEntry represents a cached repository
type RepoEntry struct {
	URL       string    // GitHub repository URL
	LocalPath string    // Local filesystem path
	BaseSHA   string    // Base commit SHA
	HeadSHA   string    // Head commit SHA
	FetchedAt time.Time // When it was cached
}
