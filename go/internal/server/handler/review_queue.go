package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	gh "github.com/google/go-github/v84/github"
	"github.com/itterno/bistro/internal/git"
	"github.com/itterno/bistro/internal/provider"
	storedb "github.com/itterno/bistro/internal/store/db"
	"github.com/itterno/bistro/internal/ws"
)

const reviewQueueTimeout = 30 * time.Second
const reviewQueueError = "Unable to read review requests. Try again later."

// ReviewQueueDeps binds queue reads to the run and provider lifetimes.
type ReviewQueueDeps struct {
	Context     context.Context
	GetProvider func() (provider.Provider, context.Context)
	DB          *sql.DB
	Broadcast   func(any)
	Log         *slog.Logger
}

// NewRequestReviewQueueHandler starts queue discovery without blocking the socket.
// The returned wait function seals registration and joins all queue workers.
func NewRequestReviewQueueHandler(deps ReviewQueueDeps) (request ws.MessageHandler, wait func()) {
	var lifecycle sync.Mutex
	var workers sync.WaitGroup
	isClosing := false
	wait = func() {
		lifecycle.Lock()
		isClosing = true
		lifecycle.Unlock()
		workers.Wait()
	}
	request = func(_ context.Context, msg ws.InboundMessage) (interface{}, error) {
		if err := validateReviewQueuePayload(msg.Data); err != nil {
			deps.Log.Error("invalid review queue payload", "request_id", msg.ID, "error", err)
			return reviewQueueErrorResponse(msg.ID, reviewQueueError), nil
		}
		prov, providerCtx := deps.GetProvider()
		if prov == nil || providerCtx == nil {
			return reviewQueueErrorResponse(msg.ID, "not authenticated"), nil
		}
		queueProvider, ok := prov.(provider.ReviewQueueProvider)
		if !ok {
			deps.Log.Error("provider does not support review queue", "request_id", msg.ID)
			return reviewQueueErrorResponse(msg.ID, reviewQueueError), nil
		}
		lifecycle.Lock()
		if isClosing || deps.Context.Err() != nil {
			lifecycle.Unlock()
			return nil, nil
		}
		workers.Add(1)
		lifecycle.Unlock()
		go func() {
			defer workers.Done()
			readReviewQueue(providerCtx, deps, queueProvider, msg.ID)
		}()
		return nil, nil
	}
	return request, wait
}

func validateReviewQueuePayload(payload json.RawMessage) error {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return nil
	}
	if payload[0] != '{' {
		return errors.New("review queue payload must be an empty object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return errors.New("review queue payload is invalid JSON")
	}
	if len(fields) != 0 {
		return errors.New("review queue payload must be an empty object")
	}
	return nil
}

func readReviewQueue(providerCtx context.Context, deps ReviewQueueDeps, queueProvider provider.ReviewQueueProvider, requestID string) {
	searchCtx, stopSearch := context.WithTimeout(providerCtx, reviewQueueTimeout)
	defer stopSearch()
	detach := context.AfterFunc(deps.Context, stopSearch)
	defer detach()
	queue, err := fetchAndPersistReviewQueue(searchCtx, deps.DB, queueProvider)
	if deps.Context.Err() != nil || providerCtx.Err() != nil {
		return
	}
	if err != nil {
		deps.Log.Error("read review queue failed", "request_id", requestID, "error", reviewQueueLogError(err))
		deps.Broadcast(reviewQueueErrorResponse(requestID, reviewQueueError))
		return
	}
	deps.Broadcast(map[string]any{"id": requestID, "type": "REVIEW_QUEUE_READY", "data": queue})
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

func reviewQueueErrorResponse(requestID, message string) map[string]any {
	return map[string]any{"id": requestID, "type": "REVIEW_QUEUE_READY", "data": map[string]string{"error": message}}
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
		identity, parseErr := git.ParseRemoteURL("https://github.com/" + repository.Owner + "/" + repository.Name)
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
