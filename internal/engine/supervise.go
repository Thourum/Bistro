package engine

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/itterno/bistro/internal/services"
)

const maxLoopBackoff = 30 * time.Second

type supervision struct {
	name     string
	reporter services.PanicReporter
	log      *slog.Logger
	sleep    func(time.Duration)
}

func supervise(ctx context.Context, opts supervision, fn func(context.Context)) {
	backoff := time.Second
	for ctx.Err() == nil {
		if !runGuarded(ctx, opts, fn) {
			return
		}
		if ctx.Err() != nil {
			return
		}
		opts.sleep(backoff)
		backoff = min(backoff*2, maxLoopBackoff)
	}
}

func runGuarded(ctx context.Context, opts supervision, fn func(context.Context)) (panicked bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicked = true
			stack := debug.Stack()
			opts.log.Error("background loop panicked; restarting", "loop", opts.name, "stack", string(stack))
			if opts.reporter != nil {
				func() {
					defer func() { _ = recover() }()
					opts.reporter.ReportPanic(opts.name, recovered, stack)
				}()
			}
		}
	}()
	fn(ctx)
	return false
}

func (a *App) startLoop(ctx context.Context, name string, fn func(context.Context)) {
	a.loops.Add(1)
	go func() {
		defer a.loops.Done()
		supervise(ctx, supervision{
			name: name, reporter: a.panics, log: a.log,
			sleep: func(duration time.Duration) {
				timer := time.NewTimer(duration)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-ctx.Done():
				}
			},
		}, fn)
	}()
}
