package engine

import (
	"sync"
	"time"

	"github.com/itterno/bistro/internal/services"
)

// syncStatusTracker derives the tray/renderer sync state from sync-pass
// boundaries and the outcome events those passes emit. Publishing runs outside
// mu so listeners can never deadlock the sync loop.
type syncStatusTracker struct {
	mu         sync.Mutex
	now        func() time.Time
	afterFunc  func(time.Duration, func()) (stop func() bool)
	publish    func(services.SyncStatus)
	inFlight   int
	batch      syncBatch
	isSignedIn bool
	isAutoSync bool
	lastSynced time.Time
	lastError  string
	resumeAt   time.Time
	stopResume func() bool
	status     services.SyncStatus
}

type syncBatch struct {
	isCompleted bool
	errorCode   string
	resumeAt    time.Time
}

func newSyncStatusTracker(publish func(services.SyncStatus)) *syncStatusTracker {
	return &syncStatusTracker{
		now:     time.Now,
		publish: publish,
		afterFunc: func(d time.Duration, f func()) func() bool {
			return time.AfterFunc(d, f).Stop
		},
	}
}

// begin supersedes any pending rate-limit window: a new pass is a new attempt.
func (t *syncStatusTracker) begin() {
	t.update(func() {
		t.inFlight++
		if t.inFlight == 1 {
			t.batch = syncBatch{}
			t.clearResumeLocked()
		}
	})
}

func (t *syncStatusTracker) end() {
	t.update(func() {
		t.inFlight--
		if t.inFlight > 0 {
			return
		}
		// Outcomes are kept independently; computeLocked picks the most severe.
		// A completed pass counts as a sync even if some repos failed.
		if t.batch.isCompleted {
			t.lastSynced = t.now()
		}
		t.lastError = t.batch.errorCode
		if !t.batch.resumeAt.IsZero() {
			t.armResumeLocked(t.batch.resumeAt)
		}
	})
}

// observe sees every engine event; outcomes outside a pass apply immediately
// because rate limits arrive asynchronously through the event bridge.
func (t *syncStatusTracker) observe(name string, data any) {
	t.update(func() {
		switch payload := data.(type) {
		case services.SyncError:
			if t.inFlight > 0 {
				t.batch.errorCode = payload.ErrorCode
			} else {
				t.lastError = payload.ErrorCode
			}
		case services.RateLimited:
			resumeAt := t.now().Add(time.Duration(payload.ResetIn * float64(time.Second)))
			if t.inFlight > 0 {
				t.batch.resumeAt = resumeAt
			} else {
				t.armResumeLocked(resumeAt)
			}
		case services.SyncComplete:
			if t.inFlight > 0 {
				t.batch.isCompleted = true
			}
		}
	})
}

func (t *syncStatusTracker) setSignedIn(isSignedIn bool) {
	t.update(func() { t.isSignedIn = isSignedIn })
}

func (t *syncStatusTracker) setAutoSync(isAutoSync bool) {
	t.update(func() { t.isAutoSync = isAutoSync })
}

func (t *syncStatusTracker) current() services.SyncStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

func (t *syncStatusTracker) clearResumeLocked() {
	if t.stopResume != nil {
		t.stopResume()
		t.stopResume = nil
	}
	t.resumeAt = time.Time{}
}

func (t *syncStatusTracker) armResumeLocked(resumeAt time.Time) {
	t.clearResumeLocked()
	t.resumeAt = resumeAt
	t.stopResume = t.afterFunc(resumeAt.Sub(t.now()), func() {
		t.update(func() { t.resumeAt = time.Time{} })
	})
}

// update applies one mutation, recomputes, and publishes only real changes.
func (t *syncStatusTracker) update(mutate func()) {
	t.mu.Lock()
	mutate()
	next := t.computeLocked()
	changed := next != t.status
	t.status = next
	publish := t.publish
	t.mu.Unlock()
	if changed && publish != nil {
		publish(next)
	}
}

// computeLocked orders states by what the user must act on first.
func (t *syncStatusTracker) computeLocked() services.SyncStatus {
	status := services.SyncStatus{}
	if !t.lastSynced.IsZero() {
		status.LastSyncedAt = t.lastSynced.UTC().Format(time.RFC3339)
	}
	switch {
	case !t.isSignedIn:
		status.State = services.SyncStateSignedOut
	case t.inFlight > 0:
		status.State = services.SyncStateSyncing
	case !t.resumeAt.IsZero():
		status.State = services.SyncStateRateLimited
		status.ResumeAt = t.resumeAt.UTC().Format(time.RFC3339)
	case t.lastError != "":
		status.State = services.SyncStateError
		status.ErrorCode = t.lastError
	case !t.isAutoSync:
		status.State = services.SyncStatePaused
	default:
		status.State = services.SyncStateUpToDate
	}
	return status
}

// statusEmitter lets the tracker observe exactly what the UI receives.
type statusEmitter struct {
	inner   services.Emitter
	tracker *syncStatusTracker
}

func (e statusEmitter) Emit(name string, data any) {
	e.inner.Emit(name, data)
	e.tracker.observe(name, data)
}

func (a *App) trackedSync(run func()) {
	a.syncStatus.begin()
	defer a.syncStatus.end()
	run()
}
