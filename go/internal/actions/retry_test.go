// go/internal/actions/retry_test.go
package actions

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)

	durations := []time.Duration{
		backoff.Next(),
		backoff.Next(),
		backoff.Next(),
	}

	// Should roughly double each time (1ms, 2ms, 4ms), capped at 10ms
	if durations[0] < 1*time.Millisecond || durations[0] > 5*time.Millisecond {
		t.Errorf("first backoff out of range: %v", durations[0])
	}
	if durations[2] > 10*time.Millisecond {
		t.Errorf("backoff exceeded max: %v > 10ms", durations[2])
	}
}

func TestBackoffCapsBeforeDurationConversion(t *testing.T) {
	backoff := NewExponentialBackoff(time.Second, 5*time.Second, 2)
	if got := backoff.CalculateDelay(100); got != 5*time.Second {
		t.Fatalf("large attempt delay = %v, want 5s", got)
	}
}

func TestRetryWithBackoffClampsNonpositiveAttempts(t *testing.T) {
	called := 0
	backoff := NewExponentialBackoff(time.Millisecond, time.Second, 2)
	result := RetryWithBackoff(context.Background(), backoff, 0, func() (*ActionResult, error) {
		called++
		return &ActionResult{Success: true}, nil
	})
	if called != 1 || !result.Success || result.Retries != 0 {
		t.Fatalf("called %d times, result = %+v; want one successful attempt", called, result)
	}
}

func TestRetryWithBackoff(t *testing.T) {
	attempts := 0
	handler := func() (*ActionResult, error) {
		attempts++
		if attempts < 3 {
			return &ActionResult{
				Success: false,
				Error:   "rate limited",
			}, nil
		}
		return &ActionResult{
			Success: true,
		}, nil
	}

	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)
	result := RetryWithBackoff(
		context.Background(),
		backoff,
		3,
		handler,
	)

	if !result.Success {
		t.Error("expected success after retries")
	}
	if result.Retries != 2 {
		t.Errorf("expected 2 retries, got %d", result.Retries)
	}
}

func TestRetryMaxAttemptsExceeded(t *testing.T) {
	// Use a transient error so IsRecoverable returns true and retries are attempted.
	// This tests that the retry loop stops after maxAttempts regardless.
	handler := func() (*ActionResult, error) {
		return &ActionResult{
			Success: false,
			Error:   "rate limit exceeded",
		}, nil
	}

	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)
	result := RetryWithBackoff(
		context.Background(),
		backoff,
		2,
		handler,
	)

	if result.Success {
		t.Error("expected failure after max retries")
	}
	if result.Retries != 1 { // 1 retry = 2 attempts total
		t.Errorf("expected 1 retry, got %d", result.Retries)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancel

	handler := func() (*ActionResult, error) {
		return &ActionResult{Success: false, Error: "should not run"}, nil
	}

	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)
	result := RetryWithBackoff(ctx, backoff, 3, handler)

	if result == nil {
		t.Error("expected result even with cancelled context")
	}
	// Context cancellation should stop retries immediately
}

func TestExponentialBackoffWithJitter(t *testing.T) {
	eb := &ExponentialBackoff{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.1, // 10% jitter
	}

	// Run calculation multiple times, expect variation
	durations := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		durations[i] = eb.CalculateDelay(i)
	}

	// Check that not all durations are identical (jitter applied)
	unique := make(map[time.Duration]bool)
	for _, d := range durations {
		unique[d] = true
	}
	if len(unique) < 3 {
		t.Errorf("expected jitter to create variation, got %d unique values", len(unique))
	}
}

func TestJitterWithinBounds(t *testing.T) {
	eb := &ExponentialBackoff{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.5, // 50% jitter
	}

	for i := 0; i < 20; i++ {
		d := eb.CalculateDelay(i)
		base := time.Duration(float64(100*time.Millisecond) * math.Pow(2.0, float64(i)))
		if base > 5*time.Second {
			base = 5 * time.Second
		}

		// With 50% jitter, should be 50-150% of base
		minExpected := base / 2
		maxExpected := base + base/2

		if d < minExpected || d > maxExpected {
			t.Errorf("jitter out of bounds for attempt %d: %v not in [%v, %v]", i, d, minExpected, maxExpected)
		}
	}
}

func TestZeroJitterNoDrift(t *testing.T) {
	eb := &ExponentialBackoff{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     5 * time.Second,
		JitterFactor: 0.0, // No jitter
	}

	// Without jitter, same attempt should always give same delay
	d1 := eb.CalculateDelay(2)
	d2 := eb.CalculateDelay(2)
	if d1 != d2 {
		t.Errorf("expected deterministic delay without jitter: %v vs %v", d1, d2)
	}
}

// ── Liveness / context-cancellation guards ────────────────────────────────

// TestRetryWithBackoff_ContextCancelledDuringFn verifies that when fn() returns
// success but the context is already cancelled (liveness fired mid-call),
// RetryWithBackoff does NOT accept that as a success and instead returns
// a failure result with the cancellation error.
func TestRetryWithBackoff_ContextCancelledDuringFn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)

	result := RetryWithBackoff(ctx, backoff, 3, func() (*ActionResult, error) {
		// Cancel the context to simulate provider returning success after liveness
		// fired during the network call.
		cancel()
		return &ActionResult{
			PRID:    "pr-1",
			Action:  ApproveCommand,
			Success: true,
		}, nil
	})

	if result.Success {
		t.Error("RetryWithBackoff must not accept success when context is cancelled during fn()")
	}
	if result.Error == "" {
		t.Error("expected non-empty error on cancelled-during-fn result")
	}
}

// TestRetryWithBackoff_ContextAlreadyCancelledBeforeFn verifies the existing
// pre-fn cancellation check still works (regression guard).
func TestRetryWithBackoff_ContextAlreadyCancelledBeforeFn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before any attempts
	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)

	called := 0
	result := RetryWithBackoff(ctx, backoff, 3, func() (*ActionResult, error) {
		called++
		return &ActionResult{Success: true}, nil
	})

	if result.Success {
		t.Error("must not succeed when context cancelled before fn")
	}
	if called > 1 {
		t.Errorf("fn should be called at most once after pre-cancel, called %d times", called)
	}
}

// TestRetryWithBackoff_ContextCancelledDuringFn_SentinelPrefix verifies that
// the cancellation-after-success error carries ErrAmbiguousRemoteSuccess prefix
// so replay can detect it and mark the queue row terminal.
func TestRetryWithBackoff_ContextCancelledDuringFn_SentinelPrefix(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backoff := NewExponentialBackoff(1*time.Millisecond, 10*time.Millisecond, 2.0)

	result := RetryWithBackoff(ctx, backoff, 3, func() (*ActionResult, error) {
		cancel() // cancel mid-call, then return success
		return &ActionResult{Success: true}, nil
	})

	if result.Success {
		t.Error("must not be success when context cancelled during fn")
	}
	if !result.IsAmbiguousRemoteSuccess() {
		t.Errorf("error must carry ErrAmbiguousRemoteSuccess prefix; got: %q", result.Error)
	}
}
