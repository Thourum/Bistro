// go/internal/actions/retry.go
package actions

import (
	"context"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// ExponentialBackoff implements exponential backoff with configurable parameters.
// All fields are shared between the stateful Next() method and the stateless
// CalculateDelay method. Use NewExponentialBackoff to construct for Next() usage,
// or initialize public fields directly for CalculateDelay usage.
type ExponentialBackoff struct {
	mu           sync.Mutex
	InitialDelay time.Duration
	Multiplier   float64
	MaxDelay     time.Duration
	JitterFactor float64 // 0.0-1.0, percentage variance
	current      time.Duration
}

// CalculateDelay returns delay for attempt N with optional jitter.
func (eb *ExponentialBackoff) CalculateDelay(attempt int) time.Duration {
	base := time.Duration(float64(eb.InitialDelay) * math.Pow(eb.Multiplier, float64(attempt)))
	if base > eb.MaxDelay {
		base = eb.MaxDelay
	}

	// Apply jitter if configured
	if eb.JitterFactor > 0 {
		jitterRange := float64(base) * eb.JitterFactor
		jitterAmount := time.Duration(rand.Float64()*jitterRange - jitterRange/2)
		base = base + jitterAmount
		if base < 0 {
			base = 0
		}
	}

	return base
}

// NewExponentialBackoff creates a new backoff calculator
func NewExponentialBackoff(initial, max time.Duration, multiplier float64) *ExponentialBackoff {
	return &ExponentialBackoff{
		InitialDelay: initial,
		MaxDelay:     max,
		Multiplier:   multiplier,
		current:      initial,
	}
}

// Next returns the current backoff duration and advances internal state for the next call
func (eb *ExponentialBackoff) Next() time.Duration {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	result := eb.current

	next := time.Duration(float64(eb.current) * eb.Multiplier)
	if next > eb.MaxDelay {
		next = eb.MaxDelay
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
