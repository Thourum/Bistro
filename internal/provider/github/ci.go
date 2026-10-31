package github

import (
	"context"
	"fmt"
	"time"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
)

// getCIStatus fetches CI check runs for the given commit ref and normalizes them.
func (c *Client) getCIStatus(ctx context.Context, owner, repo, ref string) (*provider.CIStatus, error) {
	checks, _, err := c.gh.Checks.ListCheckRunsForRef(ctx, owner, repo, ref, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get CI status for %s/%s@%s: %w", owner, repo, ref, mapAPIError(err))
	}

	checkList := make([]*provider.Check, 0, len(checks.CheckRuns))
	for _, ghCheck := range checks.CheckRuns {
		checkList = append(checkList, normalizeCheckRun(ghCheck))
	}

	return &provider.CIStatus{
		Status:    aggregateStatus(checkList),
		Checks:    checkList,
		UpdatedAt: time.Now(),
	}, nil
}

// aggregateStatus derives the overall status from individual check results.
// Returns "pending" until all checks complete, then "failure" if any failed,
// or "success" if all passed.
func aggregateStatus(checks []*provider.Check) string {
	if len(checks) == 0 {
		return "pending"
	}

	for _, check := range checks {
		if check.Status == "failure" {
			return "failure"
		}
	}

	for _, check := range checks {
		if check.Status == "pending" {
			return "pending"
		}
	}

	return "success"
}

// normalizeCheckRun converts a GitHub CheckRun to the domain Check type.
// When a check is "completed", its conclusion ("success", "failure", etc.) is used as status.
// All other states map to "pending".
func normalizeCheckRun(ghCheck *ghlib.CheckRun) *provider.Check {
	status := normalizeCheckStatus(ghCheck.GetStatus(), ghCheck.GetConclusion())

	updatedAt := time.Time{}
	if ghCheck.CompletedAt != nil {
		updatedAt = ghCheck.CompletedAt.Time
	}

	return &provider.Check{
		Name:      ghCheck.GetName(),
		Status:    status,
		URL:       ghCheck.GetDetailsURL(),
		UpdatedAt: updatedAt,
	}
}

// normalizeCheckStatus maps GitHub's (status, conclusion) pair to a domain status string.
func normalizeCheckStatus(status, conclusion string) string {
	if status == "completed" {
		switch conclusion {
		case "success":
			return "success"
		case "failure", "timed_out", "cancelled", "action_required":
			return "failure"
		default:
			return "pending"
		}
	}
	return "pending"
}

// GetCIStatus implements provider.Provider, parsing the "github:owner/repo:number" ID format.
// Fetches the PR to resolve the head SHA, then retrieves check runs.
func (c *Client) GetCIStatus(ctx context.Context, prID string) (*provider.CIStatus, error) {
	owner, repo, number, err := parsePRID(prID)
	if err != nil {
		return nil, err
	}

	pr, err := c.getPR(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("GetCIStatus: failed to fetch PR to resolve head SHA: %w", err)
	}

	return c.getCIStatus(ctx, owner, repo, pr.HeadSHA)
}
