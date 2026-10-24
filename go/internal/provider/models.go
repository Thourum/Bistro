package provider

import "time"

// PR represents a normalized pull request
type PR struct {
	ID                 string // "github:owner/repo:123" format
	Provider           string // "github", "gitlab", "gitea"
	Owner              string // org name
	Repo               string // repo name
	Number             int    // PR number
	Title              string
	Description        string
	Author             string
	State              string   // "open", "closed", "merged"
	ReviewStatus       string   // "pending", "approved", "changes_requested"
	ApprovedBy         string   // username who approved
	ChangesRequestedBy string   // username who requested changes
	RequestedReviewers []string // logins whose review is currently requested
	BaseBranch         string
	HeadBranch         string
	BaseSHA            string
	HeadSHA            string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	MergedAt           *time.Time
	CommentCount       int
	CIStatus           string // "pending", "success", "failure"
	Draft              bool
}

// Review represents a normalized PR review submission.
type Review struct {
	PRID        string
	Author      string
	State       string // "approved", "changes_requested", "commented", "dismissed", "pending"
	CommitID    string // Head commit the review was submitted against.
	SubmittedAt time.Time
}

// Comment represents a normalized comment. Kind discriminates issue vs. inline
// review comments. Anchoring fields (CommitSHA … ThreadID) are populated only
// for kind='review'; they are zero/nil for kind='issue'.
type Comment struct {
	ID     string
	PRID   string
	Author string
	Body   string
	Kind   string // "issue" | "review"
	// Anchoring fields — review comments only.
	CommitSHA         string
	OriginalSHA       string
	Side              string // "LEFT" | "RIGHT"
	StartSide         string // "LEFT" | "RIGHT" for multi-line comments
	Position          *int
	OriginalPosition  *int
	OriginalLine      *int   // GitHub original_line, nil when absent
	StartLine         *int   // GitHub start_line for multi-line comments, nil when absent
	OriginalStartLine *int   // GitHub original_start_line, nil when absent
	SubjectType       string // "LINE" | "FILE" for review comments when provided
	InReplyTo         string // ID of parent comment, or "" for root
	ThreadID          string // provider-opaque thread ID
	DiffHunk          string // raw diff hunk from GitHub (review comments only; "" for issue comments)
	// PatchsetNumber is the patchset this comment was made on.
	// nil for freshly-fetched comments (not yet persisted); populated by the
	// anchor resolver from the DB so that original-position geometry uses exact
	// patchset identity (comments.patchset_number → patchsets.number).
	PatchsetNumber *int
	// Shared.
	LineNo    int    // 0 if general comment, >0 for specific line
	FilePath  string // reuses existing column; no new path column
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ReviewThread groups inline review comments under a single thread.
// StateKnown is false when GraphQL enrichment failed and resolution state
// could not be determined (REST-only fallback).
type ReviewThread struct {
	ID         string
	PRID       string
	FilePath   string
	Line       int // 0 if outdated/unknown
	Resolved   bool
	Outdated   bool
	StateKnown bool
	Comments   []*Comment // ordered; first is the root
}

// Repo represents a normalized repository
type Repo struct {
	Owner    string
	Name     string
	FullName string // "owner/repo"
	Private  bool
}

// CIStatus represents CI/check run status
type CIStatus struct {
	Status    string // "pending", "success", "failure"
	Checks    []*Check
	UpdatedAt time.Time
}

// Check represents a single check run
type Check struct {
	Name      string // e.g., "Tests", "Lint"
	Status    string // "pending", "success", "failure"
	URL       string
	UpdatedAt time.Time
}
