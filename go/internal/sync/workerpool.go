package sync

import (
	"context"
	"fmt"
	"log/slog"
	gosync "sync"
)

// DefaultWorkers is the default number of concurrent workers used in the pool.
const DefaultWorkers = 4

// WorkerPool manages a pool of goroutines that process submitted tasks concurrently.
type WorkerPool struct {
	workers   int
	taskCh    chan func()
	wg        gosync.WaitGroup
	closeOnce gosync.Once
	log       *slog.Logger
}

// NewWorkerPool creates a new worker pool with the given number of workers.
// If workerCount is <= 0, it defaults to DefaultWorkers.
// The log parameter is optional; pass nil to disable panic logging.
func NewWorkerPool(workerCount int, log *slog.Logger) *WorkerPool {
	if workerCount <= 0 {
		workerCount = DefaultWorkers
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
		return ctx.Err()
	}
}
