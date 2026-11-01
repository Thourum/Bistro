// Package logging provides minimal context helpers for propagating a
// *slog.Logger through call chains. Everything else uses stdlib log/slog
// conventions directly.
package logging

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

// WithLogger returns a derived context that carries the given logger.
// Downstream code can retrieve it via FromContext.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the logger stored on ctx, or slog.Default() if none is
// set. It never returns nil, so callers don't need to guard.
func FromContext(ctx context.Context) *slog.Logger {
	if ctx != nil {
		if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
			return l
		}
	}
	return slog.Default()
}
