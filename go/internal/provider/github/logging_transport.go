package github

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/logging"
)

// sensitiveHeaders are dropped from log output to prevent leaking secrets.
var sensitiveHeaders = [...]string{"Authorization", "Cookie", "Set-Cookie"}

// loggingTransport is an http.RoundTripper that emits structured slog records
// for every HTTP request made by the GitHub client.
//
// Logs request metadata, response status, timing, and rate limits without
// consuming bodies. Sensitive request headers are redacted.
type loggingTransport struct {
	base http.RoundTripper
}

func newLoggingTransport(base http.RoundTripper) *loggingTransport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &loggingTransport{base: base}
}

// RoundTrip implements http.RoundTripper.
func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	log := logging.FromContext(req.Context())

	// Do not log request bodies: review text and GraphQL variables can contain
	// private user content.
	startAttrs := []any{
		"method", req.Method,
		"url", req.URL.Path,
		"query", req.URL.RawQuery,
		"host", req.URL.Host,
	}
	if h := redactedHeaders(req.Header); len(h) > 0 {
		startAttrs = append(startAttrs, "headers", h)
	}
	log.Debug("github http start", startAttrs...)

	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	dur := time.Since(start).Milliseconds()
	if err != nil {
		log.Error("github http error", "method", req.Method, "url", req.URL.Path, "duration_ms", dur, "error", err)
		return nil, err
	}

	attrs := []any{
		"method", req.Method,
		"url", req.URL.Path,
		"status", resp.StatusCode,
		"duration_ms", dur,
		"ratelimit_remaining", resp.Header.Get("X-RateLimit-Remaining"),
		"ratelimit_limit", resp.Header.Get("X-RateLimit-Limit"),
		"ratelimit_reset", resp.Header.Get("X-RateLimit-Reset"),
		"etag", resp.Header.Get("ETag"),
	}

	switch {
	case resp.StatusCode == http.StatusNotModified:
		// 304s are common with conditional requests but worth surfacing because
		// they explain "outdated PRs" symptoms — the server is saying "unchanged".
		log.Warn("github http not modified", attrs...)
	case resp.StatusCode >= 400:
		log.Warn("github http non-2xx", attrs...)
	default:
		log.Debug("github http done", attrs...)
	}

	return resp, nil
}

// redactedHeaders returns a copy of h with sensitive values replaced by
// "[REDACTED]". Multi-value headers are flattened to the first value for
// log brevity.
func redactedHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) == 0 {
			continue
		}
		if slices.Contains(sensitiveHeaders[:], http.CanonicalHeaderKey(k)) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = strings.Join(vs, ",")
	}
	return out
}
