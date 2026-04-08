// go/internal/actions/retry_test.go
package actions

import (
	"context"
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
	handler := func() (*ActionResult, error) {
		return &ActionResult{
			Success: false,
			Error:   "always fails",
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
