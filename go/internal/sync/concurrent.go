package sync

import (
	"context"
	"fmt"
	"log/slog"
	gosync "sync"

	"github.com/samuelasvanyi/bistro/internal/provider"
)

// DefaultWorkers is the default number of concurrent workers used in the pool.
const DefaultWorkers = 4

// WorkerPool manages a pool of goroutines that process submitted tasks concurrently.
type WorkerPool struct {
	workers int
	taskCh  chan func()
	wg      gosync.WaitGroup
	log     *slog.Logger
}

// NewWorkerPool creates a new worker pool with the given number of workers.
// If workerCount is <= 0, it defaults to 1.
// The log parameter is optional; pass nil to disable panic logging.
func NewWorkerPool(workerCount int, log *slog.Logger) *WorkerPool {
	if workerCount <= 0 {
		workerCount = 1
	}

	pool := &WorkerPool{
		workers: workerCount,
		taskCh:  make(chan func(), workerCount*2),
		log:     log,
	}

	for i := 0; i < workerCount; i++ {
		pool.wg.Add(1)
		go pool.worker()
	}

	return pool
}

// worker drains tasks from taskCh until the channel is closed.
// Each task is executed inside a recover so that a panicking task does not kill
// the worker goroutine — the worker continues processing remaining tasks.
func (p *WorkerPool) worker() {
	defer p.wg.Done()

	for task := range p.taskCh {
		p.safeRun(task)
	}
}

// safeRun executes a single task with panic recovery so the worker survives.
func (p *WorkerPool) safeRun(task func()) {
	defer func() {
		if r := recover(); r != nil {
			if p.log != nil {
				p.log.Error("worker panic recovered", "panic", fmt.Sprintf("%v", r))
			}
		}
	}()
	task()
}

// Submit enqueues a task for execution by the pool.
// Returns ctx.Err() immediately if the context is already done, rather than
// blocking indefinitely when the internal buffer is full.
func (p *WorkerPool) Submit(ctx context.Context, task func()) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.taskCh <- task:
		return nil
	}
}

// Wait closes the task channel and blocks until all workers finish or the
// context is cancelled, whichever comes first.
// Returns ctx.Err() if the context was cancelled before all workers completed.
// The pool must not be used after Wait returns.
func (p *WorkerPool) Wait(ctx context.Context) error {
	close(p.taskCh)

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// PRFetchResult holds the fetched data for a single PR, including any
// per-PR error encountered during concurrent fetching.
type PRFetchResult struct {
	PR       *provider.PR
	Comments []*provider.Comment
	CI       *provider.CIStatus
	DiffSize int64
	Error    error
}

// FetchPRDetails fetches comments, CI status, and diff size for each PR
// using a pool of DefaultWorkers concurrent workers. Results are returned in the same
// order as the input prs slice. Per-PR errors are captured in PRFetchResult.Error
// and do not abort fetching of other PRs. The returned error is non-nil only for
// fatal pool failures; individual PR errors are stored in PRFetchResult.Error.
func (o *Orchestrator) FetchPRDetails(
	ctx context.Context,
	prs []*provider.PR,
	includeComments, includeCI, includeDiff bool,
) ([]*PRFetchResult, error) {
	results := make([]*PRFetchResult, len(prs))
	var mu gosync.Mutex

	pool := NewWorkerPool(DefaultWorkers, nil)

	for i, pr := range prs {
		idx := i
		prCopy := pr

		task := func() {
			result := &PRFetchResult{PR: prCopy}

			var errs []string

			if includeComments && o.provider != nil {
				comments, err := o.provider.GetComments(ctx, prCopy.ID)
				if err != nil {
					errs = append(errs, fmt.Sprintf("comments: %v", err))
					if o.log != nil {
						o.log.Error("comment fetch failed", "pr_id", prCopy.ID, "err", err)
					}
				} else {
					result.Comments = comments
				}
			}

			if includeCI && o.provider != nil {
				ci, err := o.provider.GetCIStatus(ctx, prCopy.ID)
				if err != nil {
					errs = append(errs, fmt.Sprintf("ci: %v", err))
					if o.log != nil {
						o.log.Error("ci fetch failed", "pr_id", prCopy.ID, "err", err)
					}
				} else {
					result.CI = ci
				}
			}

			if includeDiff && o.git != nil {
				diffResult, err := o.git.GenerateDiff("data/repos/temp", prCopy.BaseSHA, prCopy.HeadSHA)
				if err != nil {
					errs = append(errs, fmt.Sprintf("diff: %v", err))
					if o.log != nil {
						o.log.Error("diff generation failed", "pr_id", prCopy.ID, "err", err)
					}
				} else if diffResult != nil {
					result.DiffSize = diffResult.Size
				}
			}

			if len(errs) > 0 {
				combined := ""
				for i, e := range errs {
					if i > 0 {
						combined += "; "
					}
					combined += e
				}
				result.Error = fmt.Errorf("%s", combined)
			}

			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}

		if err := pool.Submit(ctx, task); err != nil {
			// Context cancelled — record error for this PR and stop submitting.
			mu.Lock()
			results[idx] = &PRFetchResult{PR: prCopy, Error: fmt.Errorf("task submission cancelled: %w", err)}
			mu.Unlock()
			break
		}
	}

	if err := pool.Wait(ctx); err != nil {
		// Context cancelled while draining; remaining slots stay nil.
		return results, nil
	}
	return results, nil
}

// SyncPRsWithDetails fetches the PR list then concurrently fetches all detail
// types requested in the SyncRequest. Returns one PRFetchResult per PR.
func (o *Orchestrator) SyncPRsWithDetails(
	ctx context.Context,
	req *SyncRequest,
) ([]*PRFetchResult, error) {
	if o.provider == nil {
		return nil, fmt.Errorf("provider not configured")
	}

	filters := provider.PRFilters{
		State: req.State,
		Limit: 100,
	}

	prs, err := o.provider.ListPRs(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list PRs: %w", err)
	}

	if o.log != nil {
		o.log.Info("prs fetched, fetching details concurrently", "count", len(prs))
	}

	results, err := o.FetchPRDetails(
		ctx,
		prs,
		req.IncludePRs,
		req.IncludeCI,
		req.IncludeDiff,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR details: %w", err)
	}

	return results, nil
}
