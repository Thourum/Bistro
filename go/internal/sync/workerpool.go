package sync

import (
	"context"
	"fmt"
	"log/slog"
	gosync "sync"
)

// DefaultWorkers is the default number of concurrent workers used in the pool.
const DefaultWorkers = 4

// workerQueueFactor allows one waiting task per active worker during bursts.
const workerQueueFactor = 2

// WorkerPool manages a pool of goroutines that process submitted tasks concurrently.
type WorkerPool struct {
	taskCh    chan func()
	wg        gosync.WaitGroup
	closeOnce gosync.Once
	log       *slog.Logger
}

// NewWorkerPool creates a new worker pool with the given number of workers.
// If workerCount is <= 0, it defaults to DefaultWorkers.
// If log is nil, slog.Default() is used.
func NewWorkerPool(workerCount int, log *slog.Logger) *WorkerPool {
	if workerCount <= 0 {
		workerCount = DefaultWorkers
	}
	if log == nil {
		log = slog.Default()
	}

	pool := &WorkerPool{
		taskCh: make(chan func(), workerCount*workerQueueFactor),
		log:    log,
	}

	for i := 0; i < workerCount; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}

	return pool
}

// worker drains tasks from taskCh until the channel is closed.
// Each task is executed inside a recover so that a panicking task does not kill
// the worker goroutine — the worker continues processing remaining tasks.
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()

	for task := range p.taskCh {
		p.safeRun(id, task)
	}
}

// safeRun executes a single task with panic recovery so the worker survives.
func (p *WorkerPool) safeRun(workerID int, task func()) {
	defer func() {
		if r := recover(); r != nil {
			p.log.Error("worker panic recovered",
				"worker_id", workerID,
				"panic", fmt.Sprintf("%v", r),
			)
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
	p.closeOnce.Do(func() { close(p.taskCh) })

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		p.log.Warn("worker pool wait cancelled", "error", ctx.Err())
		return ctx.Err()
	}
}
