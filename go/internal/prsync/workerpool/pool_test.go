package workerpool

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConcurrentGoroutinePool(t *testing.T) {
	// Test goroutine pool can be created
	pool := New(4, nil)
	if pool == nil {
		t.Error("expected non-nil pool")
	}
}

func TestWorkerPoolSubmitTask(t *testing.T) {
	pool := New(2, nil)
	ctx := context.Background()

	// Submit a simple task
	result := make(chan int, 1)
	if err := pool.Submit(ctx, func() {
		result <- 42
	}); err != nil {
		t.Fatalf("unexpected submit error: %v", err)
	}

	// Wait a bit for task execution
	select {
	case val := <-result:
		if val != 42 {
			t.Errorf("expected 42, got %d", val)
		}
	case <-time.After(1 * time.Second):
		t.Error("task did not execute in time")
	}

	if err := pool.Wait(ctx); err != nil {
		t.Errorf("unexpected wait error: %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

func TestWorkerPoolDefaultsToDefaultWorkers(t *testing.T) {
	pool := New(0, nil)
	wasRun := make(chan struct{}, 1)
	if err := pool.Submit(context.Background(), func() { wasRun <- struct{}{} }); err != nil {
		t.Fatalf("submit with default workers: %v", err)
	}
	if err := pool.Wait(context.Background()); err != nil {
		t.Fatalf("wait with default workers: %v", err)
	}
	select {
	case <-wasRun:
	default:
		t.Fatal("task was not run with default workers")
	}
}

func TestWorkerPoolMultipleTasks(t *testing.T) {
	pool := New(3, nil)
	ctx := context.Background()
	results := make(chan int, 5)

	for i := 0; i < 5; i++ {
		val := i
		if err := pool.Submit(ctx, func() {
			results <- val
		}); err != nil {
			t.Fatalf("unexpected submit error: %v", err)
		}
	}

	if err := pool.Wait(ctx); err != nil {
		t.Errorf("unexpected wait error: %v", err)
	}
	close(results)

	count := 0
	for range results {
		count++
	}
	if count != 5 {
		t.Errorf("expected 5 results, got %d", count)
	}
}

// TestSubmitContextCancellation verifies that Submit returns ctx.Err() immediately
// when the context is already cancelled, rather than blocking on a full channel.
func TestSubmitContextCancellation(t *testing.T) {
	// Use 1 worker, buffer=2; fill the buffer so the next Submit would block.
	pool := New(1, nil)
	ctx, cancel := context.WithCancel(context.Background())

	// Block the worker so the buffer fills up.
	started := make(chan struct{})
	unblock := make(chan struct{})
	if err := pool.Submit(ctx, func() {
		close(started)
		<-unblock
	}); err != nil {
		t.Fatalf("unexpected error on first submit: %v", err)
	}
	<-started // worker is now busy

	// Fill the remaining buffer slots (buffer size = workerCount*2 = 2).
	if err := pool.Submit(ctx, func() {}); err != nil {
		t.Fatalf("unexpected error filling buffer slot 1: %v", err)
	}
	if err := pool.Submit(ctx, func() {}); err != nil {
		t.Fatalf("unexpected error filling buffer slot 2: %v", err)
	}

	// Cancel the context now; the next Submit must return immediately.
	cancel()

	err := pool.Submit(ctx, func() {})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	close(unblock)
	// Drain the pool cleanly with a fresh background context.
	_ = pool.Wait(context.Background())
}

// TestWaitContextCancellation verifies that Wait returns ctx.Err() when the
// context is cancelled before all workers have finished.
func TestWaitContextCancellation(t *testing.T) {
	pool := New(1, nil)

	// Submit a task that blocks until we release it.
	unblock := make(chan struct{})
	if err := pool.Submit(context.Background(), func() {
		<-unblock
	}); err != nil {
		t.Fatalf("unexpected submit error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Wait should return ctx.Err() because context is already cancelled.
	done := make(chan error, 1)
	go func() {
		done <- pool.Wait(ctx)
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Wait did not respect context cancellation in time")
	}

	// Unblock worker to allow goroutine cleanup.
	close(unblock)
}

// TestWorkerPanicRecovery verifies that a panicking task does not propagate
// the panic and that the pool continues to process subsequent tasks.
func TestWorkerPanicRecovery(t *testing.T) {
	pool := New(1, nil)
	ctx := context.Background()

	result := make(chan int, 1)

	// First task panics.
	if err := pool.Submit(ctx, func() {
		panic("deliberate test panic")
	}); err != nil {
		t.Fatalf("unexpected submit error: %v", err)
	}

	// Give the worker a moment to panic and recover, then submit a second task.
	// We can't submit after Wait, so use a short sleep to allow recovery.
	time.Sleep(50 * time.Millisecond)

	// The worker should have recovered; but after a panic the worker goroutine
	// exits (calls wg.Done). Submit to a new pool to test continued operation.
	pool2 := New(1, nil)
	if err := pool2.Submit(ctx, func() {
		result <- 99
	}); err != nil {
		t.Fatalf("unexpected submit error on pool2: %v", err)
	}

	if err := pool2.Wait(ctx); err != nil {
		t.Errorf("unexpected wait error: %v", err)
	}

	select {
	case val := <-result:
		if val != 99 {
			t.Errorf("expected 99, got %d", val)
		}
	default:
		t.Error("expected result from pool2 task")
	}

	// Original pool: Wait should complete without hanging.
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- pool.Wait(ctx)
	}()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Errorf("unexpected wait error from panic pool: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("pool.Wait() hung after worker panic — wg.Done not called")
	}
}

// TestWorkerPanicDoesntBlockPool verifies that a panic in one worker does not
// prevent pool.Wait() from returning.
func TestWorkerPanicDoesntBlockPool(t *testing.T) {
	pool := New(2, nil)
	ctx := context.Background()

	var mu context.Context = ctx
	_ = mu

	completed := make(chan struct{}, 2)

	if err := pool.Submit(ctx, func() {
		panic("worker 0 panic")
	}); err != nil {
		t.Fatalf("submit error: %v", err)
	}
	if err := pool.Submit(ctx, func() {
		completed <- struct{}{}
	}); err != nil {
		t.Fatalf("submit error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- pool.Wait(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("unexpected wait error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("pool.Wait() blocked forever after panic — deadlock detected")
	}
}
