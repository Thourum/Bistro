package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/logging"
	"github.com/itterno/bistro/internal/provider"
)

const (
	reviewQueuePageSize   = 100
	recentReviewRepoLimit = 10
	recentReviewDays      = 30
)

// GetReviewQueue reads one search page each for requested and recent reviews.
func (c *Client) GetReviewQueue(ctx context.Context, now time.Time) (*provider.ReviewQueue, error) {
	// The shared transport logs raw query strings; these contain the login.
	// Report failures once at the queue handler boundary instead.
	ctx = logging.WithLogger(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)))
	username, err := c.GetUsername(ctx)
	if err != nil {
		return nil, fmt.Errorf("get review queue username: %w", err)
	}
	if strings.TrimSpace(username) == "" {
		return nil, errors.New("review queue username is empty")
	}
	requestedQuery := "is:pr is:open archived:false review-requested:" + username
	recentQuery := "is:pr reviewed-by:" + username + " updated:>=" + now.UTC().AddDate(0, 0, -recentReviewDays).Format("2006-01-02")
	requested, _, err := c.gh.Search.Issues(ctx, requestedQuery,
		&gh.SearchOptions{ListOptions: gh.ListOptions{PerPage: reviewQueuePageSize}})
	if err != nil {
		return nil, fmt.Errorf("search requested reviews: %w", mapAPIError(err))
	}
	recent, _, err := c.gh.Search.Issues(ctx, recentQuery,
		&gh.SearchOptions{Sort: "updated", Order: "desc", ListOptions: gh.ListOptions{PerPage: reviewQueuePageSize}})
	if err != nil {
		return nil, fmt.Errorf("search recent reviews: %w", mapAPIError(err))
	}
	return collectReviewQueue(requested.Issues, recent.Issues)
}

func collectReviewQueue(requested, recent []*gh.Issue) (*provider.ReviewQueue, error) {
	queue := &provider.ReviewQueue{
		Requested: []provider.RequestedRepo{}, RecentlyReviewed: []string{},
		Repositories: []*provider.Repo{},
	}
	// Canonical IDs merge repository case variants.
	repositories := make(map[string]string)
	addRepository := func(id string, repository *provider.Repo) string {
		key := strings.ToLower(id)
		if repositories[key] == "" {
			repositories[key] = id
			queue.Repositories = append(queue.Repositories, repository)
		}
		return key
	}
	counts := make(map[string]int)
	for _, issue := range requested {
		id, repository, err := reviewQueueRepository(issue)
		if err != nil {
			return nil, fmt.Errorf("read requested review repository: %w", err)
		}
		key := addRepository(id, repository)
		counts[repositories[key]]++
	}
	for id, count := range counts {
		queue.Requested = append(queue.Requested, provider.RequestedRepo{RepoID: id, Count: count})
	}
	slices.SortFunc(queue.Requested, func(a, b provider.RequestedRepo) int {
		if a.Count != b.Count {
			return b.Count - a.Count
		}
		return strings.Compare(strings.ToLower(a.RepoID), strings.ToLower(b.RepoID))
	})
	reviewed := make(map[string]bool)
	for _, issue := range recent {
		id, repository, err := reviewQueueRepository(issue)
		if err != nil {
			return nil, fmt.Errorf("read recent review repository: %w", err)
		}
		key := addRepository(id, repository)
		if !reviewed[key] && len(queue.RecentlyReviewed) < recentReviewRepoLimit {
			queue.RecentlyReviewed = append(queue.RecentlyReviewed, repositories[key])
		}
		reviewed[key] = true
	}
	return queue, nil
}

func reviewQueueRepository(issue *gh.Issue) (string, *provider.Repo, error) {
	if issue == nil {
		return "", nil, errors.New("review queue search item is nil")
	}
	owner, name, err := ownerRepoFromIssue(issue)
	if err != nil {
		return "", nil, fmt.Errorf("read search repository identity: %w", err)
	}
	identity, err := remote.ParseURL("https://github.com/" + owner + "/" + name)
	if err != nil {
		return "", nil, fmt.Errorf("parse search repository identity: %w", err)
	}
	owner, name = strings.ToLower(identity.Owner), strings.ToLower(identity.Repo)
	return identity.ID(), &provider.Repo{Owner: owner, Name: name, FullName: owner + "/" + name}, nil
}

var _ provider.ReviewQueueProvider = (*Client)(nil)
