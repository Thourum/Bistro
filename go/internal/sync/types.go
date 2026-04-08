package sync

import "time"

// SyncRequest defines what to fetch during a sync
type SyncRequest struct {
	Provider    string // "github", "gitlab", "gitea"
	Owner       string // Repository owner
	Repo        string // Repository name
	IncludePRs  bool   // Fetch PR list
	IncludeCI   bool   // Fetch CI status for each PR
	IncludeDiff bool   // Generate diffs for each PR
	State       string // "open", "closed", "all"
}

// SyncResult contains the results of a sync operation
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

// SyncProgress tracks ongoing sync state
type SyncProgress struct {
	Stage        string // "starting", "fetching_prs", "fetching_comments", "generating_diffs", "writing_db", "complete"
	Current      int    // Current item number
	Total        int    // Total items
	PRsProcessed int
	ErrorCount   int
}
