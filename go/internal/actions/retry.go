// go/internal/actions/retry.go
package actions

import (
	"context"
	"time"
)

// ExponentialBackoff implements exponential backoff with configurable parameters
type ExponentialBackoff struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	multiplier   float64
	current      time.Duration
}

// NewExponentialBackoff creates a new backoff calculator
func NewExponentialBackoff(initial, max time.Duration, multiplier float64) *ExponentialBackoff {
	return &ExponentialBackoff{
		initialDelay: initial,
		maxDelay:     max,
		multiplier:   multiplier,
		current:      initial,
	}
}

// Next returns the current backoff duration and advances internal state for the next call
func (eb *ExponentialBackoff) Next() time.Duration {
	result := eb.current

	next := time.Duration(float64(eb.current) * eb.multiplier)
	if next > eb.maxDelay {
		next = eb.maxDelay
	}

	eb.current = next
	return result
}

// RetryFunc is a function that returns an ActionResult and optionally an error
type RetryFunc func() (*ActionResult, error)

// RetryWithBackoff retries an action with exponential backoff.
// maxAttempts is the total number of attempts (1 initial + N-1 retries).
func RetryWithBackoff(ctx context.Context, backoff *ExponentialBackoff, maxAttempts int, fn RetryFunc) *ActionResult {
	var lastResult *ActionResult

	for attempt := 0; attempt < maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			if lastResult == nil {
				lastResult = &ActionResult{
					Success: false,
					Error:   "context cancelled",
				}
			}
			return lastResult
		default:
		}

		result, err := fn()
		if err != nil {
			return &ActionResult{
				Success: false,
				Error:   err.Error(),
				Retries: attempt,
			}
		}

		lastResult = result

		if result.Success {
			result.Retries = attempt
			return result
		}

		if !result.IsRecoverable() {
			result.Retries = attempt
			return result
		}

		// Do not sleep after the last attempt
		if attempt < maxAttempts-1 {
			wait := backoff.Next()
			select {
			case <-ctx.Done():
				lastResult.Retries = attempt
				return lastResult
			case <-time.After(wait):
				// Continue to next attempt
			}
		}
	}

	// All attempts exhausted
	if lastResult != nil {
		lastResult.Retries = maxAttempts - 1
	} else {
		lastResult = &ActionResult{
			Success: false,
			Error:   "max attempts exceeded",
			Retries: maxAttempts - 1,
		}
	}

	return lastResult
}
