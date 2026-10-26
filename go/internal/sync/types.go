package sync

import "time"

// SyncRequest defines what to fetch during a sync.
type SyncRequest struct {
	Provider        string // "github", "gitlab", "gitea"
	Owner           string // Repository owner
	Repo            string // Repository name
	IncludePRs      bool   // Fetch PR list
	IncludeComments bool   // Fetch PR comments for each PR
	IncludeCI       bool   // Fetch CI status for each PR
	IncludeDiff     bool   // Generate diffs for each PR
	State           string // "open", "closed", "all"
}

// SyncResult contains the results of a sync operation.
type SyncResult struct {
	PRCount      int
	NewPRs       int
	UpdatedPRs   int
	DeletedPRs   int
	DiffCount    int
	CommentCount int
	Duration     time.Duration
	Errors       []string
}
