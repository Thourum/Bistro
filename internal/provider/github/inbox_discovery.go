package github

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	ghlib "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/provider"
)

const defaultInboxDiscoveryLimit = 100

// DiscoverInboxPRs finds PRs relevant to a user's inbox via GitHub search
// pointers (`involves:USER is:pr`) and hydrates each result through the repo PR
// endpoint. The search cursor/ETag are local sync metadata; returned PRs remain
// normalized GitHub-synced state.
func (c *Client) DiscoverInboxPRs(ctx context.Context, opts provider.InboxDiscoveryOptions) (*provider.InboxDiscoveryResult, error) {
	if strings.TrimSpace(opts.Username) == "" {
		return nil, fmt.Errorf("username required for inbox discovery")
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultInboxDiscoveryLimit
	}

	query := inboxDiscoveryQuery(opts.Username, opts.Since)
	result := &provider.InboxDiscoveryResult{}
	page := 1

	for len(result.PRs) < limit {
		etag := ""
		if page == 1 {
			etag = opts.ETag
		}
		issues, respETag, nextPage, err := c.searchInboxIssuesPage(ctx, query, page, etag)
		if err != nil {
			return nil, err
		}
		if page == 1 {
			result.ETag = respETag
		}

		for _, issue := range issues {
			if issue == nil || issue.GetPullRequestLinks() == nil {
				continue
			}
			owner, repo, err := ownerRepoFromIssue(issue)
			if err != nil {
				return nil, err
			}
			pr, err := c.getPR(ctx, owner, repo, issue.GetNumber())
			if err != nil {
				return nil, err
			}
			result.PRs = append(result.PRs, pr)
			if pr.UpdatedAt.After(result.HighWater) {
				result.HighWater = pr.UpdatedAt
			}
			if len(result.PRs) >= limit {
				break
			}
		}

		if nextPage == 0 {
			break
		}
		page = nextPage
	}

	return result, nil
}

func inboxDiscoveryQuery(username string, since *time.Time) string {
	parts := []string{"is:pr", "archived:false", "involves:" + username}
	if since != nil && !since.IsZero() {
		parts = append(parts, "updated:>="+since.UTC().Format(time.RFC3339))
	}
	return strings.Join(parts, " ")
}

func (c *Client) searchInboxIssuesPage(ctx context.Context, query string, page int, etag string) ([]*ghlib.Issue, string, int, error) {
	req, err := c.newSearchInboxIssuesRequest(query, page, etag)
	if err != nil {
		return nil, "", 0, err
	}
	return c.doSearchInboxIssuesPage(ctx, req, query, page, etag != "")
}

func (c *Client) newSearchInboxIssuesRequest(query string, page int, etag string) (*http.Request, error) {
	values := url.Values{}
	values.Set("q", query)
	values.Set("sort", "updated")
	values.Set("order", "asc")
	values.Set("per_page", "100")
	values.Set("page", strconv.Itoa(page))

	req, err := c.gh.NewRequest("GET", "search/issues?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	return req, nil
}

func (c *Client) doSearchInboxIssuesPage(ctx context.Context, req *http.Request, query string, page int, conditional bool) ([]*ghlib.Issue, string, int, error) {
	var searchResult ghlib.IssuesSearchResult
	resp, err := c.gh.Do(ctx, req, &searchResult)
	if conditional && resp != nil && resp.Response != nil && resp.Response.StatusCode == http.StatusNotModified {
		// Search results are paginated and page 1 can be unchanged while later
		// pages contain newly-appended PRs. Use the durable ETag as a validator,
		// but refetch the page unconditionally instead of short-circuiting the
		// whole discovery window.
		unconditionalReq, reqErr := c.newSearchInboxIssuesRequest(query, page, "")
		if reqErr != nil {
			return nil, "", 0, reqErr
		}
		return c.doSearchInboxIssuesPage(ctx, unconditionalReq, query, page, false)
	}
	if err != nil {
		return nil, "", 0, fmt.Errorf("search inbox issues: %w", mapAPIError(err))
	}
	respETag := ""
	nextPage := 0
	if resp != nil {
		respETag = resp.Header.Get("ETag")
		nextPage = resp.NextPage
	}
	return searchResult.Issues, respETag, nextPage, nil
}

func ownerRepoFromIssue(issue *ghlib.Issue) (string, string, error) {
	if repoURL := issue.GetRepositoryURL(); repoURL != "" {
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", "", err
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 3 && parts[len(parts)-3] == "repos" {
			return parts[len(parts)-2], parts[len(parts)-1], nil
		}
	}
	parts := strings.Split(strings.TrimPrefix(issue.GetHTMLURL(), "https://github.com/"), "/")
	if len(parts) >= 4 && parts[2] == "pull" {
		return parts[0], parts[1], nil
	}
	return "", "", fmt.Errorf("cannot determine repo for search issue %d", issue.GetNumber())
}

var _ provider.InboxDiscoveryProvider = (*Client)(nil)
