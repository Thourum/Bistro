package engine

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"
)

type panicRecord struct {
	name      string
	recovered any
	stack     []byte
}

type panicRecorder struct{ records []panicRecord }

func (r *panicRecorder) ReportPanic(name string, recovered any, stack []byte) {
	r.records = append(r.records, panicRecord{name, recovered, stack})
}

func TestSuperviseRestartsPanics(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reporter := &panicRecorder{}
	var delays []time.Duration
	calls := 0
	running := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervise(ctx, supervision{
			name: "test", reporter: reporter,
			log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			sleep: func(delay time.Duration) { delays = append(delays, delay) },
		}, func(ctx context.Context) {
			calls++
			if calls < 3 {
				panic("restart me")
			}
			close(running)
			<-ctx.Done()
		})
	}()
	select {
	case <-running:
	case <-time.After(time.Second):
		t.Fatal("loop did not restart")
	}
	cancel()
	<-done
	if calls != 3 || !reflect.DeepEqual(delays, []time.Duration{time.Second, 2 * time.Second}) || len(reporter.records) != 2 {
		t.Fatalf("calls=%d delays=%v panics=%d", calls, delays, len(reporter.records))
	}
	for _, record := range reporter.records {
		if record.name != "test" || record.recovered != "restart me" || len(record.stack) == 0 {
			t.Fatalf("panic report = %#v", record)
		}
	}
}

func TestSuperviseNormalReturn(t *testing.T) {
	calls := 0
	supervise(context.Background(), supervision{
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		sleep: func(time.Duration) { t.Fatal("slept after a normal return") },
	}, func(context.Context) { calls++ })
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestSuperviseBackoffCap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var delays []time.Duration
	supervise(ctx, supervision{
		log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		sleep: func(delay time.Duration) {
			delays = append(delays, delay)
			if len(delays) == 7 {
				cancel()
			}
		},
	}, func(context.Context) { panic("restart") })
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("delays = %v, want %v", delays, want)
	}
}
