package workerpool

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestConcurrentGoroutinePool(t *testing.T) {
	pool := New(4, nil)
	t.Cleanup(func() {
		if err := pool.Wait(context.Background()); err != nil {
			t.Fatalf("wait for idle workers: %v", err)
		}
	})
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

func TestWorkerPanicRecovery(t *testing.T) {
	var panicCount int
	pool := New(1, nil, Options{OnPanic: func(recovered any, stack []byte) {
		panicCount++
		if recovered != "deliberate test panic" || len(stack) == 0 {
			t.Errorf("bad panic report: %v", recovered)
		}
	}})
	completed := make(chan struct{}, 1)
	ctx := context.Background()
	if err := pool.Submit(ctx, func() { panic("deliberate test panic") }); err != nil {
		t.Fatal(err)
	}
	if err := pool.Submit(ctx, func() { completed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	err := pool.Wait(ctx)
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("Wait = %v, want panic task error", err)
	}
	if panicCount != 1 {
		t.Fatalf("panic reports = %d, want 1", panicCount)
	}
	select {
	case <-completed:
	default:
		t.Fatal("same worker did not complete subsequent task")
	}
}
