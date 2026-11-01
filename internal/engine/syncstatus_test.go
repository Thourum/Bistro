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
	tracker.setSignedIn(syncSignInSettings{isSignedIn: true})
	tracker.setAutoSync(syncAutoSettings{isAutoSync: true})
	return tracker, &published, timer, &clock
}

func TestSyncStatusPublishesConcurrentUpdatesInOrder(t *testing.T) {
	tracker, _, _, _ := newTestTracker(t)
	firstPublishing := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan struct{})
	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	published := make(chan services.SyncStatus, 2)
	tracker.publish = func(status services.SyncStatus) {
		if status.State == services.SyncStateSyncing {
			close(firstPublishing)
			<-releaseFirst
		}
		published <- status
	}
	go func() { defer close(firstDone); tracker.begin() }()
	<-firstPublishing
	go func() {
		defer close(secondDone)
		close(secondStarted)
		tracker.setSignedIn(syncSignInSettings{isSignedIn: false})
	}()
	<-secondStarted
	const interleavingWindow = 100 * time.Millisecond
	select {
	case <-secondDone:
		t.Error("newer update overtook the blocked publication")
	case <-time.After(interleavingWindow):
	}
	close(releaseFirst)
	<-firstDone
	<-secondDone
	if first := <-published; first.State != services.SyncStateSyncing {
		t.Errorf("first publication = %+v, want syncing", first)
	}
	if last := <-published; last != tracker.current() {
		t.Errorf("last publication = %+v, current = %+v", last, tracker.current())
	}
}

func TestSyncStatusSupersededResumeCallbackKeepsNewDeadline(t *testing.T) {
	tracker, _, _, _ := newTestTracker(t)
	var callbacks []func()
	tracker.afterFunc = func(_ time.Duration, callback func()) func() bool {
		callbacks = append(callbacks, callback)
		// The callback has already started, so stopping it cannot prevent delivery.
		return func() bool { return false }
	}
	const firstResetSeconds = 30
	const nextResetSeconds = 90
	tracker.observe(services.EventRateLimited, services.RateLimited{ResetIn: firstResetSeconds})
	tracker.observe(services.EventRateLimited, services.RateLimited{ResetIn: nextResetSeconds})
	want := tracker.current()
	callbacks[0]()
	if got := tracker.current(); got != want {
		t.Fatalf("superseded callback cleared newer deadline: got %+v, want %+v", got, want)
	}
	callbacks[1]()
	if got := tracker.current(); got.State != services.SyncStateUpToDate || got.ResumeAt != "" {
		t.Fatalf("current callback did not expire deadline: %+v", got)
	}
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
	tracker.setAutoSync(syncAutoSettings{isAutoSync: false})
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
	app.syncStatus.setSignedIn(syncSignInSettings{isSignedIn: true})
	if got := app.SyncStatus().State; got != services.SyncStatePaused {
		t.Fatalf("pass after panic = %q, want paused", got)
	}
}

func TestSyncStatusSignedOutWinsAndPublishesOnlyChanges(t *testing.T) {
	tracker, published, _, _ := newTestTracker(t)
	before := len(*published)
	tracker.setAutoSync(syncAutoSettings{isAutoSync: true}) // unchanged
	if len(*published) != before {
		t.Fatal("unchanged state was republished")
	}
	tracker.setSignedIn(syncSignInSettings{isSignedIn: false})
	tracker.begin()
	if got := tracker.current().State; got != services.SyncStateSignedOut {
		t.Fatalf("state = %q, want signed_out", got)
	}
}
