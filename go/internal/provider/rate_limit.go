package provider

import (
	"fmt"
	"strconv"
	"time"
)

// RateLimitState tracks GitHub API rate limit status.
type RateLimitState struct {
	ResetAt time.Time
}

// IsLimited returns true if rate limit is currently active.
func (r *RateLimitState) IsLimited() bool {
	return time.Now().Before(r.ResetAt)
}

// ResetIn returns duration until rate limit resets.
func (r *RateLimitState) ResetIn() time.Duration {
	return time.Until(r.ResetAt)
}

// String returns human-readable representation.
func (r *RateLimitState) String() string {
	if !r.IsLimited() {
		return "not rate limited"
	}
	return fmt.Sprintf("rate limited, resets in %v", r.ResetIn().Round(time.Second))
}

// DetectRateLimitFrom429 parses GitHub 429 response headers.
// Returns (state, detected).
func DetectRateLimitFrom429(headers map[string]string) (*RateLimitState, bool) {
	resetStr, ok := headers["X-RateLimit-Reset"]
	if !ok {
		return nil, false
	}

	resetUnix, err := strconv.ParseInt(resetStr, 10, 64)
	if err != nil {
		return nil, false
	}

	return &RateLimitState{
		ResetAt: time.Unix(resetUnix, 0),
	}, true
}
