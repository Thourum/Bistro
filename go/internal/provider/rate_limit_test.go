package provider

import (
	"strconv"
	"testing"
	"time"
)

func TestRateLimitStateIsLimitedActive(t *testing.T) {
	state := &RateLimitState{
		ResetAt: time.Now().Add(1 * time.Minute),
	}
	if !state.IsLimited() {
		t.Error("expected rate limit to be active")
	}
}

func TestRateLimitStateIsLimitedExpired(t *testing.T) {
	state := &RateLimitState{
		ResetAt: time.Now().Add(-1 * time.Minute),
	}
	if state.IsLimited() {
		t.Error("expected rate limit to be expired")
	}
}

func TestRateLimitStateResetIn(t *testing.T) {
	future := time.Now().Add(30 * time.Second)
	state := &RateLimitState{ResetAt: future}
	duration := state.ResetIn()

	if duration <= 0 || duration > 31*time.Second {
		t.Errorf("expected ~30 seconds, got %v", duration)
	}
}

func TestDetectRateLimitFrom429(t *testing.T) {
	futureUnix := time.Now().Add(5 * time.Minute).Unix()
	headers := map[string]string{
		"X-RateLimit-Reset": strconv.FormatInt(futureUnix, 10),
	}
	state, ok := DetectRateLimitFrom429(headers)
	if !ok {
		t.Fatal("expected to detect rate limit")
	}

	if state == nil || state.ResetAt.IsZero() {
		t.Error("expected valid rate limit state")
	}
}

func TestDetectRateLimitFromMissingHeaders(t *testing.T) {
	headers := map[string]string{}
	state, ok := DetectRateLimitFrom429(headers)
	if ok {
		t.Error("expected no detection with missing headers")
	}
	if state != nil {
		t.Error("expected nil state")
	}
}

func TestDetectRateLimitFromMalformedHeader(t *testing.T) {
	headers := map[string]string{
		"X-RateLimit-Reset": "not-a-number",
	}
	state, ok := DetectRateLimitFrom429(headers)
	if ok {
		t.Error("expected no detection with malformed header")
	}
	if state != nil {
		t.Error("expected nil state")
	}
}

func TestRateLimitStateString(t *testing.T) {
	state := &RateLimitState{
		ResetAt: time.Now().Add(30 * time.Second),
	}
	str := state.String()
	if str == "" {
		t.Error("expected non-empty string representation")
	}
}
