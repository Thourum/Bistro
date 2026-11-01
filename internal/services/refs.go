package services

// PRRef keeps the existing request field name across service ports.
type PRRef struct {
	PRID string `json:"pr_id"`
}

// RepoRef keeps the existing request field name across service ports.
type RepoRef struct {
	RepoID string `json:"repo_id"`
}
