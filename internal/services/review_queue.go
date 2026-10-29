package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/git/remote"
	"github.com/itterno/bistro/internal/provider"
	storedb "github.com/itterno/bistro/internal/store/db"
)

const reviewQueueTimeout = 30 * time.Second
const reviewQueueError = "Unable to read review requests. Try again later."

// ReviewQueue reads and persists discovery within caller and provider lifetimes.
type ReviewQueue struct {
	Context     context.Context
	GetProvider func() (provider.Provider, context.Context)
	DB          *sql.DB
	Log         *slog.Logger
	Panics      PanicReporter
}

// Get blocks until discovery and its repository transaction have completed.
func (s *ReviewQueue) Get(ctx context.Context) (provider.ReviewQueue, error) {
	return guard(s.Panics, "ReviewQueue.Get", func() (provider.ReviewQueue, error) {
		prov, providerCtx := s.GetProvider()
		if prov == nil || providerCtx == nil {
			return provider.ReviewQueue{}, Errorf(CodeUnauthenticated, "not authenticated")
		}
		queueProvider, ok := prov.(provider.ReviewQueueProvider)
		if !ok {
			return provider.ReviewQueue{}, Errorf(CodeInvalid, "provider does not support review queue")
		}
		searchCtx, cancel := context.WithTimeout(ctx, reviewQueueTimeout)
		defer cancel()
		detachProvider := context.AfterFunc(providerCtx, cancel)
		defer detachProvider()
		if s.Context != nil {
			detachRun := context.AfterFunc(s.Context, cancel)
			defer detachRun()
			if err := s.Context.Err(); err != nil {
				return provider.ReviewQueue{}, err
			}
		}
		if err := providerCtx.Err(); err != nil {
			return provider.ReviewQueue{}, err
		}
		queue, err := fetchAndPersistReviewQueue(searchCtx, s.DB, queueProvider)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return provider.ReviewQueue{}, err
			}
			serviceLogger(s.Log).Error("read review queue failed", "error", reviewQueueLogError(err))
			return provider.ReviewQueue{}, Errorf(CodeInternal, reviewQueueError)
		}
		return *queue, nil
	})
}

// GitHub errors include the search URL (and login) and raw response content.
func reviewQueueLogError(err error) string {
	cause := errors.Unwrap(err)
	if cause != nil {
		for _, operation := range []string{
			"fetch review queue", "get review queue username", "failed to get authenticated user",
			"search requested reviews", "search recent reviews", "read requested review repository",
			"read recent review repository", "read search repository identity", "parse search repository identity",
			"begin queue repo transaction", "parse queue repository", "insert queue repository", "commit queue repositories",
		} {
			if strings.HasPrefix(err.Error(), operation+": ") {
				return operation + ": " + reviewQueueLogError(cause)
			}
		}
		return reviewQueueLogError(cause)
	}
	var responseError *gh.ErrorResponse
	if errors.As(err, &responseError) && responseError.Response != nil {
		return fmt.Sprintf("GitHub HTTP status %d", responseError.Response.StatusCode)
	}
	return fmt.Sprintf("failure (%T)", err)
}

func fetchAndPersistReviewQueue(ctx context.Context, database *sql.DB, queueProvider provider.ReviewQueueProvider) (*provider.ReviewQueue, error) {
	queue, err := queueProvider.GetReviewQueue(ctx, time.Now())
	if err != nil {
		return nil, fmt.Errorf("fetch review queue: %w", err)
	}
	if queue == nil {
		return nil, errors.New("review queue provider returned nil")
	}
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin queue repo transaction: %w", err)
	}
	queries := storedb.New(transaction)
	for _, repository := range queue.Repositories {
		if repository == nil {
			return nil, errors.Join(errors.New("review queue repository is nil"), transaction.Rollback())
		}
		identity, parseErr := remote.ParseURL("https://github.com/" + repository.Owner + "/" + repository.Name)
		if parseErr != nil {
			return nil, errors.Join(fmt.Errorf("parse queue repository: %w", parseErr), transaction.Rollback())
		}
		if err := queries.InsertDiscoveredRepo(ctx, storedb.InsertDiscoveredRepoParams{
			ID: identity.ID(), Provider: identity.Provider,
			Owner: identity.Owner, Repo: identity.Repo,
		}); err != nil {
			return nil, errors.Join(fmt.Errorf("insert queue repository: %w", err), transaction.Rollback())
		}
	}
	if err := transaction.Commit(); err != nil {
		rollbackErr := transaction.Rollback()
		if errors.Is(rollbackErr, sql.ErrTxDone) {
			rollbackErr = nil
		}
		return nil, errors.Join(fmt.Errorf("commit queue repositories: %w", err), rollbackErr)
	}
	return queue, nil
}
