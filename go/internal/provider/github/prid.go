package github

import (
	"fmt"
	"strconv"
	"strings"
)

// parsePRID parses a PR ID in the format "github:owner/repo:number".
// Returns owner, repo, and PR number, or an error if the format is invalid.
func parsePRID(id string) (owner, repo string, number int, err error) {
	// Expected format: "github:owner/repo:number"
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 || parts[0] != "github" {
		return "", "", 0, fmt.Errorf("invalid PR ID format %q: expected github:owner/repo:number", id)
	}

	repoPath := parts[1]
	repoParts := strings.SplitN(repoPath, "/", 2)
	if len(repoParts) != 2 || repoParts[0] == "" || repoParts[1] == "" {
		return "", "", 0, fmt.Errorf("invalid PR ID format %q: owner/repo part must be non-empty", id)
	}

	number, err = strconv.Atoi(parts[2])
	if err != nil || number <= 0 {
		return "", "", 0, fmt.Errorf("invalid PR ID format %q: number must be a positive integer", id)
	}

	return repoParts[0], repoParts[1], number, nil
}
