package provider

import "time"

// PR represents a normalized pull request
type PR struct {
	ID           string // "github:owner/repo:123" format
	Provider     string // "github", "gitlab", "gitea"
	Owner        string // org name
	Repo         string // repo name
	Number       int    // PR number
	Title        string
	Description  string
	Author       string
	State        string // "open", "closed", "merged"
	ReviewStatus          string // "pending", "approved", "changes_requested"
	ApprovedBy            string // username who approved
	ChangesRequestedBy    string // username who requested changes
	RequestedReviewers    []string // logins whose review is currently requested
	BaseBranch            string
	HeadBranch   string
	BaseSHA      string
	HeadSHA      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MergedAt     *time.Time
	CommentCount int
	CIStatus     string // "pending", "success", "failure"
	Draft        bool
}

// Review represents a normalized PR review submission.
type Review struct {
	PRID        string
	Author      string
	State       string // "approved", "changes_requested", "commented", "dismissed", "pending"
	SubmittedAt time.Time
}

// Comment represents a normalized comment
type Comment struct {
	ID        string
	PRID      string
	Author    string
	Body      string
	LineNo    int    // 0 if general comment, >0 for specific line
	FilePath  string // Only if line comment
	CreatedAt time.Time
	UpdatedAt time.Time
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
