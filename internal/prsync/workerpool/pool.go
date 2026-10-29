package workerpool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
)

// DefaultWorkers is the default number of concurrent workers used in the pool.
const DefaultWorkers = 4

// workerQueueFactor allows one waiting task per active worker during bursts.
const workerQueueFactor = 2

// Pool manages a pool of goroutines that process submitted tasks concurrently.
type Pool struct {
	taskCh     chan func()
	wg         sync.WaitGroup
	closeOnce  sync.Once
	log        *slog.Logger
	onPanic    func(any, []byte)
	errorsMu   sync.Mutex
	taskErrors []error
}

// Options supplies optional reporting without coupling workers to services.
type Options struct {
	OnPanic func(recovered any, stack []byte)
}

// New creates a new worker pool with the given number of workers.
// If workerCount is <= 0, it defaults to DefaultWorkers.
// If log is nil, slog.Default() is used.
func New(workerCount int, log *slog.Logger, options ...Options) *Pool {
	if workerCount <= 0 {
		workerCount = DefaultWorkers
	}
	if log == nil {
		log = slog.Default()
	}

	pool := &Pool{
		taskCh: make(chan func(), workerCount*workerQueueFactor),
		log:    log,
	}

	if len(options) > 0 {
		pool.onPanic = options[0].OnPanic
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
func (p *Pool) worker(id int) {
	defer p.wg.Done()

	for task := range p.taskCh {
		if err := p.safeRun(id, task); err != nil {
			p.errorsMu.Lock()
			p.taskErrors = append(p.taskErrors, err)
			p.errorsMu.Unlock()
		}
	}
}

// safeRun executes a single task with panic recovery so the worker survives.
func (p *Pool) safeRun(workerID int, task func()) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("worker task panicked: %v", recovered)
			stack := debug.Stack()
			p.log.Error("worker panic recovered", "worker_id", workerID, "stack", string(stack))
			if p.onPanic != nil {
				// A failing reporter must not kill a recovered worker.
				func() {
					defer func() { _ = recover() }()
					p.onPanic(recovered, stack)
				}()
			}
		}
	}()
	task()
	return nil
}

// Submit enqueues a task for execution by the pool.
// Returns ctx.Err() immediately if the context is already done, rather than
// blocking indefinitely when the internal buffer is full.
func (p *Pool) Submit(ctx context.Context, task func()) error {
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
func (p *Pool) Wait(ctx context.Context) error {
	p.closeOnce.Do(func() { close(p.taskCh) })

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.errorsMu.Lock()
		defer p.errorsMu.Unlock()
		return errors.Join(p.taskErrors...)
	case <-ctx.Done():
		p.log.Warn("worker pool wait cancelled", "error", ctx.Err())
		return ctx.Err()
	}
}
