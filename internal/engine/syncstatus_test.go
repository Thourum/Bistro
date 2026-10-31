package engine

import (
	"testing"
	"time"

	"github.com/itterno/bistro/internal/services"
)

type fakeTimer struct{ fire func() }

func newTestTracker(t *testing.T) (*syncStatusTracker, *[]services.SyncStatus, *fakeTimer, *time.Time) {
	t.Helper()
	var published []services.SyncStatus
	clock := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	timer := &fakeTimer{}
	tracker := newSyncStatusTracker(func(s services.SyncStatus) { published = append(published, s) })
	tracker.now = func() time.Time { return clock }
	tracker.afterFunc = func(_ time.Duration, f func()) func() bool { timer.fire = f; return func() bool { return true } }
	tracker.setSignedIn(true)
	tracker.setAutoSync(true)
	return tracker, &published, timer, &clock
}

func TestSyncStatusFailedPassThenRecovery(t *testing.T) {
	tracker, _, _, clock := newTestTracker(t)
	tracker.begin()
	tracker.begin() // nested pass keeps syncing until the outermost end
	tracker.observe(services.EventSyncError, services.SyncError{ErrorCode: "SYNC_FAILED"})
	tracker.observe(services.EventSyncComplete, services.SyncComplete{})
	tracker.end()
	if got := tracker.current().State; got != services.SyncStateSyncing {
		t.Fatalf("state after inner end = %q, want syncing", got)
	}
	tracker.end()
	firstSync := clock.UTC().Format(time.RFC3339)
	if got := tracker.current(); got.State != services.SyncStateError || got.ErrorCode != "SYNC_FAILED" || got.LastSyncedAt != firstSync {
		t.Fatalf("partially failed pass = %+v, want error with last_synced_at", got)
	}
	tracker.begin()
	tracker.observe(services.EventSyncComplete, services.SyncComplete{})
	tracker.end()
	want := clock.UTC().Format(time.RFC3339)
	if got := tracker.current(); got.State != services.SyncStateUpToDate || got.LastSyncedAt != want {
		t.Fatalf("recovered pass = %+v", got)
	}
}

func TestSyncStatusRateLimitRetriesAfterReset(t *testing.T) {
	tracker, _, timer, clock := newTestTracker(t)
	tracker.begin()
	tracker.observe(services.EventSyncError, services.SyncError{ErrorCode: "SYNC_FAILED"})
	tracker.observe(services.EventRateLimited, services.RateLimited{ResetIn: 90})
	tracker.end()
	want := clock.Add(90 * time.Second).UTC().Format(time.RFC3339)
	if got := tracker.current(); got.State != services.SyncStateRateLimited || got.ResumeAt != want {
		t.Fatalf("rate limited = %+v, want resume %s", got, want)
	}
	timer.fire()
	if got := tracker.current(); got.State != services.SyncStateError || got.ResumeAt != "" {
		t.Fatalf("after reset = %+v, want the batch's other failure", got)
	}
	tracker.begin()
	tracker.observe(services.EventSyncComplete, services.SyncComplete{})
	tracker.end()
	tracker.setAutoSync(false)
	if got := tracker.current().State; got != services.SyncStatePaused {
		t.Fatalf("clean pass with auto-sync off = %q, want paused", got)
	}
}

func TestSyncStatusListenerPanicDoesNotInterruptPass(t *testing.T) {
	app := &App{}
	app.syncStatus = newSyncStatusTracker(app.publishSyncStatus)
	reporter := &panicRecorder{}
	app.SetPanicReporter(reporter)
	const listenerPanic = "listener failed"
	app.OnSyncStatus(func(services.SyncStatus) { panic(listenerPanic) })
	app.trackedSync(func() {
		app.syncStatus.observe(services.EventSyncComplete, services.SyncComplete{})
	})
	if got := app.SyncStatus(); got.State != services.SyncStateSignedOut || got.LastSyncedAt == "" {
		t.Fatalf("pass interrupted by listener panic: %+v", got)
	}
	if len(reporter.records) == 0 {
		t.Fatal("listener panic was not reported")
	}
	for _, record := range reporter.records {
		if record.name != "sync-status-listener" || record.recovered != listenerPanic || len(record.stack) == 0 {
			t.Fatalf("panic report = %+v", record)
		}
	}
	app.OnSyncStatus(nil)
	app.syncStatus.setSignedIn(true)
	if got := app.SyncStatus().State; got != services.SyncStatePaused {
		t.Fatalf("pass after panic = %q, want paused", got)
	}
}

func TestSyncStatusSignedOutWinsAndPublishesOnlyChanges(t *testing.T) {
	tracker, published, _, _ := newTestTracker(t)
	before := len(*published)
	tracker.setAutoSync(true) // unchanged
	if len(*published) != before {
		t.Fatal("unchanged state was republished")
	}
	tracker.setSignedIn(false)
	tracker.begin()
	if got := tracker.current().State; got != services.SyncStateSignedOut {
		t.Fatalf("state = %q, want signed_out", got)
	}
}
