package github

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/itterno/bistro/internal/logging"
)

// maxLoggedBodyBytes is the per-body cap when logging request/response bodies
// at Debug level. Keeps debug output usable and bounds memory.
const maxLoggedBodyBytes = 4096

// sensitiveHeaders are dropped from log output to prevent leaking secrets.
var sensitiveHeaders = map[string]struct{}{
	"Authorization": {},
	"Cookie":        {},
	"Set-Cookie":    {},
}

// loggingTransport is an http.RoundTripper that emits structured slog records
// for every HTTP request made by the GitHub client.
//
// Logging levels:
//   - Debug: every request start, every 2xx/3xx response (with timing + ratelimit)
//   - Warn:  non-2xx/3xx responses (4xx, 5xx, 304) — the interesting failures
//   - Error: transport-level errors (DNS, TLS, timeouts)
//
// Response bodies are not consumed; only method/url/status/duration and
// rate-limit headers are logged. Authorization/Cookie headers are redacted.
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

	// At Debug, emit a "start" record with redacted headers and a size-capped
	// request body preview (if any).
	startAttrs := []any{
		"method", req.Method,
		"url", req.URL.Path,
		"query", req.URL.RawQuery,
		"host", req.URL.Host,
	}
	if req.Body != nil && req.ContentLength > 0 && req.ContentLength <= maxLoggedBodyBytes {
		if preview, err := peekRequestBody(req); err == nil && len(preview) > 0 {
			startAttrs = append(startAttrs, "request_body", string(preview))
		}
	}
	if h := redactedHeaders(req.Header); len(h) > 0 {
		startAttrs = append(startAttrs, "headers", h)
	}
	log.Debug("github http start", startAttrs...)

	start := time.Now()
	resp, err := t.base.RoundTrip(req)
	dur := time.Since(start).Milliseconds()

	if err != nil {
		log.Error("github http error",
			"method", req.Method,
			"url", req.URL.Path,
			"duration_ms", dur,
			"error", err,
		)
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

// peekRequestBody reads, then re-attaches, req.Body so it can still be sent.
// Returns at most maxLoggedBodyBytes bytes.
func peekRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	buf, err := io.ReadAll(io.LimitReader(req.Body, maxLoggedBodyBytes+1))
	if err != nil {
		return nil, err
	}
	// Always restore, even if truncated below — the original stream was drained.
	// The caller relies on req.Body remaining readable.
	req.Body.Close() //nolint:errcheck // original body is fully drained; nothing actionable on close error
	req.Body = io.NopCloser(bytes.NewReader(buf))
	if len(buf) > maxLoggedBodyBytes {
		return append(buf[:maxLoggedBodyBytes:maxLoggedBodyBytes], []byte("...(truncated)")...), nil
	}
	return buf, nil
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
		if _, redact := sensitiveHeaders[http.CanonicalHeaderKey(k)]; redact {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = strings.Join(vs, ",")
	}
	return out
}
