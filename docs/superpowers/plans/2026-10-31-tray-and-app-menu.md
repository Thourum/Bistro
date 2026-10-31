# System Tray, Background Mode and App Menu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: subagent-driven-development task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** a cross-platform tray icon shows the sync engine's state. Closing the window hides Bistro into the tray when a tray is available. A fuller native app menu dispatches the app's existing keyboard actions.

**Architecture:**
- An engine-side tracker derives a `SyncStatus` by watching the engine's own emitter and the sync-pass boundaries. It publishes `sync:status` to the renderer and to a Go listener.
- A pure `internal/tray` model turns that status into a tray view. A thin Wails adapter applies the view.
- Background mode and the app menu are wired in `package main`. They use only Wails built-ins: `SystemTray`, `Menu`, and the `pkg/services/dock` service.
- Menu items that are renderer actions are enabled by the renderer, which owns `isActive`. They are dispatched by `actionId`, so the menu can never do more than a key press could.

**Tech Stack:** Go 1.26.1, Wails v3.0.0 (`application`, `events`, `pkg/services/dock`), React 19 + TypeScript, Vitest, Biome.

**Spec:** `docs/superpowers/specs/2026-10-31-tray-and-app-menu-design.md`

## Global Constraints

- Use only Wails built-ins. Tray calls are limited to `SetIcon`, `SetTooltip`, `SetMenu` and `OnClick`. No `SetLabel`, no `SetTemplateIcon`, and no custom cgo or OS-specific code of our own.
- Dock visibility goes only through `github.com/wailsapp/wails/v3/pkg/services/dock`: `dock.New()`, `HideAppIcon()` and `ShowAppIcon()`.
  - Both use `dispatch_sync` onto the main queue (`dock_darwin.go:10-19`), so **never call them on the main thread**. Always call them from a goroutine.
  - Do not `RegisterService` the dock service. That would add it to the generated bindings.
- Menu items that dispatch an `actionId` get **no native accelerator**. Built-in roles keep theirs.
- `--smoke` creates no tray and installs no close hook.
- Use background mode (close hides) only when the tray is ready. Wails v3.0.0 only logs a failed StatusNotifierItem registration on Linux (`systemtray_linux.go:659-666`) and gives the app no signal. Wails gives no creation-failure signal on darwin or windows either. So `isTrayReady = trayWasCreated && isTrayReadyOn(runtime.GOOS)`: true on darwin and windows when `startTray` ran (not smoke); **always false on linux**, where the tray still shows but closing the window quits.
- The `SyncStatus` states, exact values: `signed_out`, `syncing`, `up_to_date`, `paused`, `rate_limited`, `error`.
- Tray status lines, exact text:
  - `Signed out`
  - `Syncing…`
  - `Up to date · synced HH:MM` (or `Up to date` before the first sync)
  - `Paused (auto-sync off)`
  - `Rate limited · retry after HH:MM`
  - `Sync error (<error_code>)`
- Tooltip: `Bistro — <status line>`.
- Tray menu, in order:
  - the status line (disabled);
  - a separator;
  - `Show Bistro`;
  - `Sync Now`, disabled while `signed_out` or `syncing`;
  - `Check for Updates…`, disabled in dev builds;
  - `Open Logs`;
  - a separator;
  - `Quit Bistro`.
- The menu dispatches exactly these action IDs: `global.settings`, `global.inbox`, `global.search`, `global.command`, `global.help`.
- External URLs: `https://get-bistro.com/docs` and `https://github.com/Thourum/Bistro/issues/new`. Both go through `services.Shell.OpenExternal`.
- Conventions from AGENTS.md:
  - Frontend uses `@thourum/fault` with no `try`/`catch`. Engine calls go only through `call()` / `callWrite()`.
  - Never hand-edit `frontend/bindings/`. After any bound-method or event change, run `wails3 task generate` and commit the bindings.
  - No React component tests. Only tests that fail if the logic breaks.
- Coding standards:
  - no positional boolean parameters (use options structs);
  - lookup tables instead of comparison chains;
  - name every meaningful literal;
  - functions of about 50 lines or fewer;
  - no dead code.
- Verification, run before every commit:
  - from the repo root, with `~/go/bin` on PATH: `go vet ./... && go test -race ./...`;
  - from `frontend/`: `pnpm type-check && pnpm lint && pnpm test`.
- Commits: `type: description`. Use the normal pre-commit hook, never `--no-verify`, and never push.

## Rulings made while planning (deviations from the spec text)

1. **Renderer-reported menu enablement replaces the spec's "disable while signed out" rule.** `global.settings` is active only on `list` / `detail`, and `global.help` / `global.command` only on `list` / `detail` / `diff` (`frontend/src/utils/keybindings.ts:97-145`). Under the spec's rule, an item would be enabled on the diff and settings pages and do nothing. The renderer reports which menu action IDs `isActive` permits through a new bound `Shell.SetMenuActions`, which keeps the spec's promise that an enabled item always does something. While signed out, the login route activates none of them, so the spec's behaviour falls out of this rule. Cost if wrong: one extra binding call per route or overlay change.
2. **New package `internal/tray`, not `internal/shell/traymodel.go`.** `services` imports `shell`, and the model needs `services.SyncStatus`, so a model inside `shell` would be an import cycle. The app-menu builder and background wiring live in `package main` (`appmenu.go`, `background.go`) for the same reason. Cost: none.
3. **`SyncStatus` timestamps are `string` with `omitempty` instead of `*string`.** The struct then stays comparable, so the tracker publishes only real changes. Cost: the TS type has optional strings instead of `string | null`.
4. **No one-minute tray ticker.** The tray shows absolute `HH:MM` times, and the tracker's own timer handles the end of a rate limit, so a ticker would be dead work. `TrayViewOptions` drops `Now`. Cost: none.
5. **One full-colour icon per state instead of separate light and dark assets.** The app-icon art reads on both menu-bar backgrounds, so there are no `SetDarkModeIcon` calls. Cost if wrong: a later task adds monochrome dark variants.
6. **`global.inbox` is a catalog entry with an activation but no key in any style table.** `getEffectiveBindings` drops actions that have no key, and palette-only entries in `ACTIONS` carry no activation. A catalog entry gives `isActive` something to evaluate, and it lets users bind a key later. Cost: none.
7. **The sync loop is serial**, since every pass runs inside `runSyncLoop`. The in-flight counter still exists, but the spec's "overlapping syncs" test becomes "nested begin/end".

---

### Task 1: Engine sync status tracker and the `sync:status` event

**Files:**
- Create: `internal/engine/syncstatus.go`
- Create: `internal/engine/syncstatus_test.go`
- Modify:
  - `internal/services/events.go` (add the type, constants and registration);
  - `internal/services/sync.go` (add `Status`);
  - `internal/engine/app.go` (add the fields, `SetEmitter`, `Start`, and the new methods `SyncStatus` and `OnSyncStatus`);
  - `internal/engine/options.go` (construct the tracker in `New`);
  - `internal/engine/syncloop.go` (wrap passes; feed auto-sync);
  - `internal/engine/provider.go` (feed signed-in);
  - `internal/engine/services.go` (wire `Sync.CurrentStatus`).
- Modify:
  - `frontend/src/services/events/index.ts` and `frontend/src/services/events/sync.ts` (the `sync:status` handler);
  - `frontend/src/store/ui.ts` (add `engineSyncStatus`);
  - `frontend/bindings/` (regenerated).

**Interfaces:**
- Produces, in `services`:
  ```go
  const (
      SyncStateSignedOut   = "signed_out"
      SyncStateSyncing     = "syncing"
      SyncStateUpToDate    = "up_to_date"
      SyncStatePaused      = "paused"
      SyncStateRateLimited = "rate_limited"
      SyncStateError       = "error"
  )
  const EventSyncStatus = "sync:status"
  type SyncStatus struct {
      State        string `json:"state"`
      LastSyncedAt string `json:"last_synced_at,omitempty"` // RFC 3339
      ResumeAt     string `json:"resume_at,omitempty"`      // RFC 3339, rate_limited only
      ErrorCode    string `json:"error_code,omitempty"`
  }
  func (s *Sync) Status(ctx context.Context) (SyncStatus, error) // field: CurrentStatus func() SyncStatus
  ```
- Produces, in `engine`:
  - `func (a *App) SyncStatus() services.SyncStatus`
  - `func (a *App) OnSyncStatus(listener func(services.SyncStatus))`. This may be called before or after `Start`. The listener runs outside the tracker's lock and may be called while `provMu` is held, so it must not call back into the engine synchronously.

- [ ] **Step 1: Add the services types.**
  1. In `internal/services/events.go`, add the constants, `EventSyncStatus` and `SyncStatus` from the interfaces block above. Register the event beside the other sync events, as `application.RegisterEvent[SyncStatus](EventSyncStatus)`.
  2. In `internal/services/sync.go`, add the field `CurrentStatus func() SyncStatus` to `Sync`, and add this method:

```go
// Status reports the engine's current sync state for first paint.
func (s *Sync) Status(ctx context.Context) (SyncStatus, error) {
	return guard(s.Panics, "Sync.Status", func() (SyncStatus, error) {
		if err := ctx.Err(); err != nil {
			return SyncStatus{}, err
		}
		if s.CurrentStatus == nil {
			return SyncStatus{}, Errorf(CodeInternal, "sync status unavailable")
		}
		return s.CurrentStatus(), nil
	})
}
```

- [ ] **Step 2: Implement the tracker** in `internal/engine/syncstatus.go`:

```go
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
```

- [ ] **Step 3: Wire the tracker into the engine.**
  - **`app.go`.** Add the fields `syncStatus *syncStatusTracker`, `rawEmitter services.Emitter`, `syncStatusMu sync.Mutex` and `syncStatusListener func(services.SyncStatus)`.
    - `SetEmitter(emitter)` stores `a.rawEmitter = emitter` and `a.emitter = statusEmitter{inner: emitter, tracker: a.syncStatus}`.
    - Add the two methods:
      ```go
      // SyncStatus returns the tracker's current state for first paint.
      func (a *App) SyncStatus() services.SyncStatus { return a.syncStatus.current() }

      // OnSyncStatus registers the shell listener; it must not call the engine synchronously.
      func (a *App) OnSyncStatus(listener func(services.SyncStatus)) {
      	a.syncStatusMu.Lock()
      	defer a.syncStatusMu.Unlock()
      	a.syncStatusListener = listener
      }
      ```
    - The tracker's publish function, set in `New`, does three things:
      1. emits `services.EventSyncStatus` via `a.rawEmitter`, if it is non-nil;
      2. reads `a.syncStatusListener` under `syncStatusMu`;
      3. calls the listener outside that lock, inside a `defer func() { if r := recover(); r != nil { ... } }()` block. On a panic, that block calls `a.panics.ReportPanic("sync-status-listener", r, debug.Stack())` when `a.panics` is non-nil. Per the spec, a panicking listener must never take down the sync loop.
    - In `Start`, after `initializeStartupProvider`, read `a.autoSync` under `syncSettingsMu`, then call `a.syncStatus.setAutoSync(...)` and `a.syncStatus.setSignedIn(a.getProvider() != nil)`.
  - **`options.go`.** In `New`, construct `a.syncStatus = newSyncStatusTracker(a.publishSyncStatus)`, where `publishSyncStatus` is a small method holding the publish logic above. If `New` currently assigns a default emitter, route it through `SetEmitter` so the wrapping always applies.
  - **`syncloop.go`.**
    - In `runSyncLoop`, wrap the three sync call sites in `a.trackedSync(...)`:
      - `a.syncPendingWatchedRepos(ctx)`;
      - the ticker's `a.syncWatchedRepos(ctx)`;
      - the trigger branch's `if repoID == "" {...} else {...}`.
    - Wherever `settings.autoSync` changes (in `applySyncSettings`) and at loop start, call `a.syncStatus.setAutoSync(settings.autoSync)`.
  - **`provider.go`.** At the end of `teardownProviderLocked`, call `a.syncStatus.setSignedIn(false)`. At the end of `attachOrchestratorLocked`, and in `SetProvider`'s non-nil branch after `a.prov = prov`, call `a.syncStatus.setSignedIn(true)`.
  - **`services.go`.** Set `Sync: &services.Sync{Trigger: a.syncTrigger, CurrentStatus: a.syncStatus.current, Panics: a.panics, Log: a.log}`.

- [ ] **Step 4: Write the behaviour tests** in `internal/engine/syncstatus_test.go`. They use a fake clock and a fake `afterFunc`:

```go
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
```

- [ ] **Step 5: Regenerate the bindings and add the renderer handler.**
  1. Run `PATH=$HOME/go/bin:$PATH wails3 task generate`.
  2. In `frontend/src/store/ui.ts`, add `engineSyncStatus: SyncStatus | null` to `UIState`, and `engineSyncStatus: null` to its initial state. Import `SyncStatus` from `@bindings/services/models`.
  3. In `frontend/src/services/events/sync.ts`, add:
     ```ts
     export function handleSyncStatus(data: Readonly<SyncStatus>): void {
       appStore.setState((state) => ({ ...state, engineSyncStatus: data }));
     }
     ```
  4. In `frontend/src/services/events/index.ts`, add `'sync:status': handleSyncStatus,` in alphabetical position.

- [ ] **Step 6: Verify.** Run `go vet ./... && go test -race ./...` and `cd frontend && pnpm type-check && pnpm lint && pnpm test`. Expected: all pass.

- [ ] **Step 7: Commit.**

```bash
git add internal/services/events.go internal/services/sync.go internal/engine frontend/bindings frontend/src/store/ui.ts frontend/src/services/events
git commit -m "feat: track engine sync status and emit sync:status"
```

---

### Task 2: Pure tray view model (`internal/tray`)

**Files:**
- Create: `internal/tray/model.go`
- Create: `internal/tray/model_test.go`

**Interfaces:**
- Consumes the `services.SyncStatus` constants from Task 1.
- Produces:
  ```go
  type Icon string   // IconIdle "idle", IconSyncing "syncing", IconAttention "attention", IconSignedOut "signed_out"
  type ItemID string // ItemStatus, ItemSeparator, ItemShow, ItemSyncNow, ItemCheckUpdates, ItemOpenLogs, ItemQuit
  type Item struct { ID ItemID; Label string; Enabled bool }
  type View struct { Icon Icon; Tooltip, StatusLine string; Items []Item }
  type ViewOptions struct { IsDevBuild bool; Location *time.Location }
  func BuildView(status services.SyncStatus, opts ViewOptions) View
  ```

- [ ] **Step 1: Implement** `internal/tray/model.go`:

```go
// Package tray turns engine sync status into a cross-platform tray icon.
package tray

import (
	"fmt"
	"time"

	"github.com/itterno/bistro/internal/services"
)

type Icon string

const (
	IconIdle      Icon = "idle"
	IconSyncing   Icon = "syncing"
	IconAttention Icon = "attention"
	IconSignedOut Icon = "signed_out"
)

type ItemID string

const (
	ItemStatus       ItemID = "status"
	ItemSeparator    ItemID = "separator"
	ItemShow         ItemID = "show"
	ItemSyncNow      ItemID = "sync_now"
	ItemCheckUpdates ItemID = "check_updates"
	ItemOpenLogs     ItemID = "open_logs"
	ItemQuit         ItemID = "quit"
)

type Item struct {
	ID      ItemID
	Label   string
	Enabled bool
}

type View struct {
	Icon       Icon
	Tooltip    string
	StatusLine string
	Items      []Item
}

type ViewOptions struct {
	IsDevBuild bool
	Location   *time.Location
}

const (
	tooltipPrefix = "Bistro — "
	clockLayout   = "15:04"
)

var stateIcons = map[string]Icon{
	services.SyncStateSignedOut:   IconSignedOut,
	services.SyncStateSyncing:     IconSyncing,
	services.SyncStateUpToDate:    IconIdle,
	services.SyncStatePaused:      IconIdle,
	services.SyncStateRateLimited: IconAttention,
	services.SyncStateError:       IconAttention,
}

var unsyncableStates = map[string]bool{
	services.SyncStateSignedOut: true,
	services.SyncStateSyncing:   true,
}

// BuildView is pure so every platform shares one tested tray behaviour.
func BuildView(status services.SyncStatus, opts ViewOptions) View {
	line := statusLine(status, opts.Location)
	icon, ok := stateIcons[status.State]
	if !ok {
		icon = IconIdle
	}
	return View{
		Icon: icon, Tooltip: tooltipPrefix + line, StatusLine: line,
		Items: []Item{
			{ID: ItemStatus, Label: line},
			{ID: ItemSeparator},
			{ID: ItemShow, Label: "Show Bistro", Enabled: true},
			{ID: ItemSyncNow, Label: "Sync Now", Enabled: !unsyncableStates[status.State]},
			{ID: ItemCheckUpdates, Label: "Check for Updates…", Enabled: !opts.IsDevBuild},
			{ID: ItemOpenLogs, Label: "Open Logs", Enabled: true},
			{ID: ItemSeparator},
			{ID: ItemQuit, Label: "Quit Bistro", Enabled: true},
		},
	}
}

func statusLine(status services.SyncStatus, location *time.Location) string {
	switch status.State {
	case services.SyncStateSignedOut:
		return "Signed out"
	case services.SyncStateSyncing:
		return "Syncing…"
	case services.SyncStatePaused:
		return "Paused (auto-sync off)"
	case services.SyncStateRateLimited:
		return withClock("Rate limited · retry after ", status.ResumeAt, location, "Rate limited")
	case services.SyncStateError:
		return fmt.Sprintf("Sync error (%s)", status.ErrorCode)
	default:
		return withClock("Up to date · synced ", status.LastSyncedAt, location, "Up to date")
	}
}

// withClock falls back when the engine has no timestamp yet.
func withClock(prefix, timestamp string, location *time.Location, fallback string) string {
	parsed, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fallback
	}
	if location == nil {
		location = time.Local
	}
	return prefix + parsed.In(location).Format(clockLayout)
}
```

- [ ] **Step 2: Write the table test** in `internal/tray/model_test.go`:

```go
package tray

import (
	"testing"
	"time"

	"github.com/itterno/bistro/internal/services"
)

func TestBuildView(t *testing.T) {
	const at = "2026-10-31T18:04:00Z"
	for _, test := range []struct {
		name        string
		status      services.SyncStatus
		isDevBuild  bool
		wantLine    string
		wantIcon    Icon
		wantSync    bool
		wantUpdates bool
	}{
		{"signed out", services.SyncStatus{State: services.SyncStateSignedOut}, false, "Signed out", IconSignedOut, false, true},
		{"syncing", services.SyncStatus{State: services.SyncStateSyncing}, false, "Syncing…", IconSyncing, false, true},
		{"up to date", services.SyncStatus{State: services.SyncStateUpToDate, LastSyncedAt: at}, false, "Up to date · synced 18:04", IconIdle, true, true},
		{"never synced", services.SyncStatus{State: services.SyncStateUpToDate}, false, "Up to date", IconIdle, true, true},
		{"paused", services.SyncStatus{State: services.SyncStatePaused}, false, "Paused (auto-sync off)", IconIdle, true, true},
		{"rate limited", services.SyncStatus{State: services.SyncStateRateLimited, ResumeAt: at}, false, "Rate limited · retry after 18:04", IconAttention, true, true},
		{"error", services.SyncStatus{State: services.SyncStateError, ErrorCode: "SYNC_FAILED"}, false, "Sync error (SYNC_FAILED)", IconAttention, true, true},
		{"dev build", services.SyncStatus{State: services.SyncStateUpToDate}, true, "Up to date", IconIdle, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := BuildView(test.status, ViewOptions{IsDevBuild: test.isDevBuild, Location: time.UTC})
			if view.StatusLine != test.wantLine || view.Tooltip != "Bistro — "+test.wantLine || view.Icon != test.wantIcon {
				t.Fatalf("view = %+v", view)
			}
			if got := enabled(view, ItemSyncNow); got != test.wantSync {
				t.Fatalf("sync now enabled = %v", got)
			}
			if got := enabled(view, ItemCheckUpdates); got != test.wantUpdates {
				t.Fatalf("check updates enabled = %v", got)
			}
			if view.Items[0].ID != ItemStatus || view.Items[0].Enabled {
				t.Fatal("status line must be the first, disabled item")
			}
		})
	}
}

func enabled(view View, id ItemID) bool {
	for _, item := range view.Items {
		if item.ID == id {
			return item.Enabled
		}
	}
	return false
}
```

- [ ] **Step 3: Verify.** Run `go vet ./... && go test -race ./internal/tray/...`. Expected: PASS.

- [ ] **Step 4: Commit.**

```bash
git add internal/tray
git commit -m "feat: add pure tray view model for sync status"
```

---

### Task 3: Tray adapter, icons, a manual update check, and wiring

**Files:**
- Create:
  - `internal/tray/tray.go`;
  - `internal/tray/icons/gen.go` (`//go:build ignore`);
  - `internal/tray/icons/{idle,syncing,attention,signed_out}.png`, generated.
- Modify: `internal/shell/updater.go` (add `CheckNow`; make `Stop` drain it).
- Create: `background.go` in the repo root, `package main`, holding the tray wiring helper. Task 4 extends this file.
- Modify: `main.go` (create the tray when not in smoke).

**Interfaces:**
- Consumes `tray.BuildView`, `tray.View`, `tray.ViewOptions` and `tray.Item*` (Task 2), and `engine.App.SyncStatus` / `OnSyncStatus` (Task 1).
- Produces:
  ```go
  // package tray
  type Actions struct { Show, SyncNow, CheckUpdates, OpenLogs, Quit func() }
  func New(app *application.App, actions Actions, opts ViewOptions, initial services.SyncStatus) *Tray
  func (t *Tray) Update(status services.SyncStatus) // safe from any goroutine
  // package shell
  func (u *UpdateChecks) CheckNow() bool // false when updates are disabled (dev) or stopping
  ```

- [ ] **Step 1: Generate the icons.**
  - Write `internal/tray/icons/gen.go`, with `//go:build ignore` and `package main`. Use only the standard library. It must:
    1. read `build/appicon.png`;
    2. box-filter it down to a 44×44 RGBA image (22pt at 2×);
    3. write four PNGs.
  - The four PNGs:
    - `idle.png`: the plain icon.
    - `syncing.png`: the icon plus a filled 12px-diameter circle in the bottom-right corner, in the design system's primary colour.
    - `attention.png`: the same circle in the error/warning colour.
    - `signed_out.png`: the icon converted to greyscale at 50% alpha.
  - Read the exact hex values from `docs/DESIGN.md` and name them as constants with a comment citing the token.
  - Run it once, from the repo root: `go run internal/tray/icons/gen.go`. Commit the generator and the PNGs.

- [ ] **Step 2: Implement the adapter** in `internal/tray/tray.go`:

```go
package tray

import (
	"embed"

	"github.com/itterno/bistro/internal/services"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed icons/*.png
var iconFiles embed.FS

// Actions are the shell operations tray menu items trigger.
type Actions struct {
	Show, SyncNow, CheckUpdates, OpenLogs, Quit func()
}

// Tray applies Views with tray calls Wails implements on every platform.
type Tray struct {
	tray    *application.SystemTray
	actions Actions
	opts    ViewOptions
}

// New must run before app.Run so Wails' defaults bind right-click to the menu.
func New(app *application.App, actions Actions, opts ViewOptions, initial services.SyncStatus) *Tray {
	t := &Tray{tray: app.SystemTray.New(), actions: actions, opts: opts}
	t.tray.OnClick(actions.Show)
	t.apply(BuildView(initial, opts))
	return t
}

// Update is safe from engine goroutines; UI work runs on the main thread.
func (t *Tray) Update(status services.SyncStatus) {
	view := BuildView(status, t.opts)
	application.InvokeAsync(func() { t.apply(view) })
}

func (t *Tray) apply(view View) {
	if icon, err := iconFiles.ReadFile("icons/" + string(view.Icon) + ".png"); err == nil {
		t.tray.SetIcon(icon)
	}
	t.tray.SetTooltip(view.Tooltip)
	menu := application.NewMenu()
	for _, item := range view.Items {
		if item.ID == ItemSeparator {
			menu.AddSeparator()
			continue
		}
		entry := menu.Add(item.Label).SetEnabled(item.Enabled)
		if handler := t.handler(item.ID); handler != nil {
			entry.OnClick(func(*application.Context) { handler() })
		}
	}
	t.tray.SetMenu(menu)
}

func (t *Tray) handler(id ItemID) func() {
	return map[ItemID]func(){
		ItemShow: t.actions.Show, ItemSyncNow: t.actions.SyncNow,
		ItemCheckUpdates: t.actions.CheckUpdates, ItemOpenLogs: t.actions.OpenLogs, ItemQuit: t.actions.Quit,
	}[id]
}
```

  The implementer must check one thing first: does `app.SystemTray.New()` before `app.Run` lead Wails to call `Run()` on the tray? Read `pkg/application` (the `SystemTrayManager` and application run code). If it does not, call `Run()` from an `events.Common.ApplicationStarted` handler instead, and record the answer in the report.

- [ ] **Step 3: Add `CheckNow`** in `internal/shell/updater.go`.
  - Add these fields to `UpdateChecks`: `app *application.App`, `ctx context.Context`, `log *slog.Logger`, `isStopping bool` and `manual sync.WaitGroup`.
  - In `Start`, after the derived `ctx` is created, store `u.app`, `u.ctx` and `u.log`.
  - In `Stop`, set `u.isStopping = true` under `mu` before reading `cancel`/`done`. After `<-done`, call `u.manual.Wait()`.
  - Add the method:

```go
// CheckNow runs one user-requested check with Wails' updater window, which
// also shows the "up to date" panel. False means updates are off in this build.
func (u *UpdateChecks) CheckNow() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.app == nil || u.isStopping || u.ctx.Err() != nil {
		return false
	}
	app, ctx, log := u.app, u.ctx, u.log
	u.manual.Add(1)
	go func() {
		defer u.manual.Done()
		if err := app.Updater.CheckAndInstall(ctx); err != nil && ctx.Err() == nil {
			log.Warn("manual update check failed", "err", err)
		}
	}()
	return true
}
```

- [ ] **Step 4: Wire the tray** in the new `background.go` (repo root, `package main`):

```go
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/itterno/bistro/internal/engine"
	"github.com/itterno/bistro/internal/services"
	"github.com/itterno/bistro/internal/shell"
	"github.com/itterno/bistro/internal/tray"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type trayDeps struct {
	app     *application.App
	eng     *engine.App
	set     *services.Set
	updates *shell.UpdateChecks
	show    func()
	ctx     context.Context
	log     *slog.Logger
}

func startTray(deps trayDeps) *tray.Tray {
	logIfFailed := func(action string, err error) {
		if err != nil {
			deps.log.Warn("tray action failed", "action", action, "err", err)
		}
	}
	icon := tray.New(deps.app, tray.Actions{
		Show: deps.show,
		SyncNow: func() {
			go func() {
				_, err := deps.set.Sync.Force(deps.ctx, services.ForceSyncRequest{})
				logIfFailed("sync_now", err)
			}()
		},
		CheckUpdates: func() {
			if !deps.updates.CheckNow() {
				deps.log.Info("update check unavailable in this build")
			}
		},
		OpenLogs: func() { go func() { logIfFailed("open_logs", deps.set.Shell.OpenLogFolder(deps.ctx)) }() },
		Quit:     deps.app.Quit,
	}, tray.ViewOptions{IsDevBuild: version == "dev", Location: time.Local}, deps.eng.SyncStatus())
	deps.eng.OnSyncStatus(icon.Update)
	return icon
}
```

  In `main.go`, after `setMenu(app)` and before `armSmoke`, add the following. Task 4 replaces `show` with the dock-aware version.

```go
if !*smoke {
	startTray(trayDeps{app: app, eng: eng, set: set, updates: updates, show: focusMainWindow, ctx: ctx, log: log})
}
```

  **Sync Now failure semantics (spec error table).** `Sync.Force` only queues the pass. It returns an error only when its context is cancelled or a panic is recovered, and it returns `Queued: false` when a pass is already queued. Real sync failures arrive as `sync:error` / `sync:rate-limited` and reach the tracker through `statusEmitter`. That is the "tracker moves to error" path. A `Queued: false` result changes nothing visible, which is correct, because the pending pass is already the user's sync. Log it at info.

- [ ] **Step 5: Verify.**
  - Run the Go and frontend checks from the Global Constraints, then `PATH=$HOME/go/bin:$PATH wails3 task build`.
  - Run `bin/bistro --smoke` if the build produces it; otherwise use the path in the Taskfile. Smoke must still pass and must show no tray.
  - Expected: all pass.

- [ ] **Step 6: Commit.**

```bash
git add internal/tray internal/shell/updater.go background.go main.go
git commit -m "feat: show sync status in a cross-platform tray icon"
```

---

### Task 4: Background mode (close hides into the tray)

**Files:**
- Modify:
  - `background.go` (close decision, close hook, dock-aware show);
  - `main.go` (dock instance; single-instance and reopen use show; install the hook);
  - `window.go` (`focusMainWindow` is reused; don't change its body).
- Create: `background_test.go`

**Interfaces:**
- Consumes `startTray` / `trayDeps` (Task 3).
- Produces:
  ```go
  type closeContext struct { isTrayReady, isSmoke bool }
  type closeAction int // closeQuit, closeHide
  func decideCloseAction(c closeContext) closeAction
  func isTrayReadyOn(goos string) bool
  func showMainWindow(dk *dock.DockService)
  func installCloseToTray(window *application.WebviewWindow, dk *dock.DockService)
  ```

- [ ] **Step 1: Implement** this in `background.go`:

```go
type closeAction int

const (
	closeQuit closeAction = iota
	closeHide
)

type closeContext struct {
	isTrayReady bool
	isSmoke     bool
}

// Wails v3.0.0 only logs a failed StatusNotifierItem registration on Linux
// (systemtray_linux.go:659-666), so there is no readiness signal there and
// hiding could strand the app with no way back.
func isTrayReadyOn(goos string) bool { return goos != "linux" }

func decideCloseAction(c closeContext) closeAction {
	if c.isSmoke || !c.isTrayReady {
		return closeQuit
	}
	return closeHide
}

// The dock service dispatch_syncs onto the main queue, so it must never run on
// the main thread where window hooks and tray clicks are delivered.
func showMainWindow(dk *dock.DockService) {
	go func() {
		dk.ShowAppIcon()
		application.InvokeAsync(focusMainWindow)
	}()
}

func installCloseToTray(window *application.WebviewWindow, dk *dock.DockService) {
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		window.Hide()
		go dk.HideAppIcon()
	})
}
```

- [ ] **Step 2: Wire it** in `main.go`. Make Task 3's `if !*smoke { startTray(...) }` keep the result in a variable (`var trayIcon *tray.Tray`) so the close decision uses whether the tray was actually created.
  - Create `dk := dock.New()` before `application.New`. Do not register it as a service.
  - Change the single-instance callback to `func(application.SecondInstanceData) { showMainWindow(dk) }`.
  - After `window := newMainWindow(...)`:

```go
if decideCloseAction(closeContext{isTrayReady: trayIcon != nil && isTrayReadyOn(runtime.GOOS), isSmoke: *smoke}) == closeHide {
	installCloseToTray(window, dk)
}
app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
	showMainWindow(dk)
})
```

  - Pass `show: func() { showMainWindow(dk) }` to `startTray`.
  - Keep `ApplicationShouldTerminateAfterLastWindowClosed: true`. A hidden window is not closed, so this still applies only when the close is not intercepted.

- [ ] **Step 3: Write the decision test** in `background_test.go`:

```go
func TestDecideCloseAction(t *testing.T) {
	for _, test := range []struct {
		name string
		ctx  closeContext
		want closeAction
	}{
		{"tray ready hides", closeContext{isTrayReady: true}, closeHide},
		{"no tray quits", closeContext{isTrayReady: false}, closeQuit},
		{"smoke never intercepts", closeContext{isTrayReady: true, isSmoke: true}, closeQuit},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := decideCloseAction(test.ctx); got != test.want {
				t.Fatalf("decideCloseAction(%+v) = %v, want %v", test.ctx, got, test.want)
			}
		})
	}
	if isTrayReadyOn("linux") || !isTrayReadyOn("darwin") || !isTrayReadyOn("windows") {
		t.Fatal("tray readiness must be false only on linux")
	}
}
```

- [ ] **Step 4: Verify.** Run the Go checks, then `wails3 task build`. Smoke must still exit cleanly. Expected: all pass.

- [ ] **Step 5: Commit.**

```bash
git add background.go background_test.go main.go
git commit -m "feat: hide Bistro into the tray when its window closes"
```

---

### Task 5: Native app menu (Go side) and the `menu:action` event

**Files:**
- Create: `appmenu.go` in the repo root, `package main`. It replaces `setMenu` in `main.go`.
- Modify:
  - `internal/shell/settings.go` (add `Update`);
  - `internal/services/events.go` (add `EventMenuAction` and `MenuAction`);
  - `internal/services/shell.go` (add `SetMenuActions` and the field `OnMenuActions`);
  - `main.go`.
- Modify:
  - `frontend/src/services/events/menu.ts` (new) and `frontend/src/services/events/index.ts`;
  - `frontend/src/store/ui.ts`;
  - `frontend/bindings/` (regenerated).

**Interfaces:**
- Consumes `showMainWindow` (Task 4) and `UpdateChecks.CheckNow` (Task 3).
- Produces:
  ```go
  // services
  const EventMenuAction = "menu:action"
  type MenuAction struct { ActionID string `json:"action_id"`; Sequence int `json:"sequence"` }
  type MenuActionsRequest struct { Enabled []string `json:"enabled"` }
  var MenuActionIDs = []string{"global.settings", "global.inbox", "global.search", "global.command", "global.help"}
  func (s *Shell) SetMenuActions(ctx context.Context, req MenuActionsRequest) error // field: OnMenuActions func(enabled []string)
  // shell
  func (s *SettingsStore) Update(mutate func(*Settings)) (Settings, error)
  ```
- Renderer:
  - `UIState.menuActionRequests: readonly { actionId: string; sequence: number }[]`;
  - `handleMenuAction(data: Readonly<MenuAction>): void`.

- [ ] **Step 1: Add `SettingsStore.Update`.** It applies one leaf change on top of the latest settings, under the same locks as `Set`, so a menu toggle never overwrites a renderer edit that is in flight:

```go
// Update mutates the latest snapshot under the change lock; shell-owned window
// bounds are preserved like Set.
func (s *SettingsStore) Update(mutate func(*Settings)) (Settings, error) {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()
	s.mu.Lock()
	next := cloneSettings(s.current)
	mutate(&next)
	next.Window = s.current.Window
	return s.commit(next)
}
```

- [ ] **Step 2: Add the services pieces.**
  - In `events.go`, add `EventMenuAction`, `MenuAction` and `MenuActionIDs`, and register the event as `application.RegisterEvent[MenuAction](EventMenuAction)`.
  - `Sequence` identifies each click; the renderer queues requests (below), so none is lost even if two arrive before React renders.
  - In `shell.go`, add the field `OnMenuActions func(enabled []string)` and this method:

```go
// SetMenuActions receives which menu actions the renderer's activation rules
// permit right now; unknown ids are dropped so the menu stays a closed set.
func (s *Shell) SetMenuActions(ctx context.Context, req MenuActionsRequest) error {
	return guardErr(s.Panics, "Shell.SetMenuActions", func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		known := make(map[string]bool, len(MenuActionIDs))
		for _, id := range MenuActionIDs {
			known[id] = true
		}
		enabled := make([]string, 0, len(req.Enabled))
		for _, id := range req.Enabled {
			if known[id] {
				enabled = append(enabled, id)
			}
		}
		if s.OnMenuActions != nil {
			s.OnMenuActions(enabled)
		}
		return nil
	})
}
```

- [ ] **Step 3: Build the menu** in `appmenu.go`.
  - **Structure.**
    - `type appMenu struct` holds the `*application.Menu` and `actionItems map[string]*application.MenuItem`, plus the theme radio items and the density checkbox.
    - `func buildAppMenu(deps appMenuDeps) *appMenu`.
    - `appMenuDeps` holds:
      - `app`;
      - `settings *shell.SettingsStore`;
      - `set *services.Set`;
      - `updates *shell.UpdateChecks`;
      - `show func()`;
      - `ctx`;
      - `log`;
      - `isDevBuild bool`.
    - Keep each function at about 50 lines or fewer: one builder per submenu.
  - **Action items.** `addAction(menu, label, actionID)` creates an item with **no accelerator**, starts it disabled, and records it in `actionItems`. Its click handler:
    1. calls `deps.show()`;
    2. increments a per-menu sequence counter, a field on `appMenu` guarded by a mutex;
    3. emits `deps.app.Event.Emit(services.EventMenuAction, services.MenuAction{ActionID: actionID, Sequence: seq})`.
  - **Bistro / File menus.**
    - On darwin, build a `Bistro` submenu containing:
      - `AddRole(application.About)`;
      - `Settings…` → `global.settings`;
      - `Check for Updates…` (native `deps.updates.CheckNow()`; hidden when `isDevBuild`);
      - a separator;
      - `AddRole(application.ServicesMenu)`;
      - a separator;
      - `AddRole(application.Hide)`, `AddRole(application.HideOthers)` and `AddRole(application.UnHide)`;
      - a separator;
      - `AddRole(application.Quit)`.
    - On other platforms, put `Settings…`, `Check for Updates…`, a separator and `Quit` (role) at the end of `File`. Put `About` (role) at the end of `Help`, after a separator, which is the Windows/Linux convention.
  - **File:**
    - `Sync Now` (native; in a goroutine: `deps.set.Sync.Force(deps.ctx, services.ForceSyncRequest{})`, logging any error);
    - `Inbox` → `global.inbox`;
    - `Search` → `global.search`;
    - `Command Palette` → `global.command`.
  - **Edit:** `AddRole(application.EditMenu)`.
  - **View:**
    - a `Theme` submenu with the radio items `Light` and `Dark`. Each calls `deps.settings.Update(func(s *shell.Settings) { s.Appearance.Theme = value })`. The values are the named constants `themeLight = "light"` and `themeDark = "dark"`, which match `settings_leaves.go:38`.
    - a `Compact Density` checkbox, which toggles `s.Appearance.HighDensity`;
    - a separator;
    - the roles `Reload`, `ResetZoom`, `ZoomIn`, `ZoomOut` and `ToggleFullscreen`.
  - **Window:** `AddRole(application.WindowMenu)`.
  - **Help:**
    - `Keyboard Shortcuts` → `global.help`;
    - `Documentation` (native `deps.set.Shell.OpenExternal(deps.ctx, docsURL)`);
    - `Report an Issue` (native `OpenExternal(issuesURL)`);
    - `Open Logs` (native `OpenLogFolder`).

    Name the URLs as constants: `docsURL = "https://get-bistro.com/docs"` and `issuesURL = "https://github.com/Thourum/Bistro/issues/new"`. Run each native call in a goroutine and log any error.
  - **`func (m *appMenu) setEnabledActions(enabled []string)`.** Inside `application.InvokeAsync`, set every item in `actionItems` to enabled only if its ID is in `enabled`, then call `m.menu.Update()`.
  - **`func (m *appMenu) syncAppearance(settings shell.Settings)`.** Inside `application.InvokeAsync`, set the radio checks from `settings.Appearance.Theme` and the checkbox from `HighDensity`, then call `m.menu.Update()`.
  - **Wiring in `main.go`.** Delete `setMenu` and replace its call with:

```go
menu := buildAppMenu(appMenuDeps{app: app, settings: settings, set: set, updates: updates,
	show: func() { showMainWindow(dk) }, ctx: ctx, log: log, isDevBuild: version == "dev"})
app.Menu.SetApplicationMenu(menu.menu)
menu.syncAppearance(settings.Get())
set.Shell.OnMenuActions = menu.setEnabledActions
```

    In the existing `settings.OnChange` listener, add `menu.syncAppearance(next)`. The listener is registered before the menu exists, so either move the menu construction above `settings.OnChange`, or register a second `settings.OnChange(func(_, next shell.Settings) { menu.syncAppearance(next) })` after it. Pick the one that keeps `main.go` readable.

    `set.Shell` is assigned before `registerServices`, so `OnMenuActions` must be set before the first renderer call. Assign it right after building the menu, which must come before `app.Run`.

- [ ] **Step 4: Regenerate the bindings and add the minimal renderer handler.**
  - Run `wails3 task generate`.
  - In `store/ui.ts`, add `menuActionRequests: readonly { actionId: string; sequence: number }[]` (initially `[]`).
  - Create `frontend/src/services/events/menu.ts`:

```ts
import type { MenuAction } from '@bindings/services/models';
import { appStore } from '../../store';

/** Menu clicks queue as store requests; useAppHotkeys drains them with the live action context. */
export function handleMenuAction(data: Readonly<MenuAction>): void {
  appStore.setState((state) => ({
    ...state,
    menuActionRequests: [
      ...state.menuActionRequests,
      { actionId: data.action_id, sequence: data.sequence },
    ],
  }));
}
```

  - Add `'menu:action': handleMenuAction,` to the table in `services/events/index.ts`.

- [ ] **Step 5: Verify.** Run the Go and frontend checks and `wails3 task build`. Expected: all pass. The renderer does not run requests yet; that is Task 6.

- [ ] **Step 6: Commit.**

```bash
git add appmenu.go main.go internal/shell/settings.go internal/services frontend/bindings frontend/src/services/events frontend/src/store/ui.ts
git commit -m "feat: add native app menu dispatching renderer actions by id"
```

---

### Task 6: Renderer menu availability, dispatch, and `global.inbox`

**Files:**
- Modify:
  - `frontend/src/utils/keybindings.ts` (add a `global.inbox` catalog entry; no keymap entries);
  - `frontend/src/utils/keybindingActions.ts` (add the `global.inbox` handler);
  - `frontend/src/utils/keybindingActivation.ts` (widen the `isActive` parameter type);
  - `frontend/src/hooks/useAppHotkeys.ts` (call `useMenuActions`).
- Create:
  - `frontend/src/utils/menuActions.ts` (pure availability);
  - `frontend/src/services/menu.ts` (reports to the engine);
  - `frontend/src/hooks/useMenuActions.ts`;
  - `frontend/src/utils/menuActions.test.ts`.

**Interfaces:**
- Consumes:
  - `UIState.menuActionRequests` (Task 5);
  - the generated `Shell.SetMenuActions` binding (Task 5);
  - `getBindingAction`, `ACTION_CATALOG` and `isActive`.
- Produces:
  ```ts
  export const MENU_ACTION_IDS = ['global.settings', 'global.inbox', 'global.search', 'global.command', 'global.help'] as const;
  export function enabledMenuActions(context: { routeBase: RouteBase | null; activeOverlay: OverlayKind | null }): string[];
  export function runMenuAction(actionId: string, context: KeyActionContext): boolean;
  export function reportMenuActions(enabled: readonly string[]): ResultAsync<void, Fault>;
  export function useMenuActions(activation: BindingActivationContext, makeCtx: () => KeyActionContext): void;
  ```

- [ ] **Step 1: Add `global.inbox`.**
  - In `GLOBAL_BINDINGS`, add:
    ```ts
    { actionId: 'global.inbox', description: 'Go to Inbox', activation: { routes: ['list', 'detail', 'diff', 'settings'] } },
    ```
  - Add no entry to any `KEYMAPS` style.
  - In `ACTIONS` (keybindingActions.ts), add `'global.inbox': () => { void router.navigate({ to: '/prs' }); },` next to `'global.settings'`.
  - "Go to Inbox" **must appear in the command palette** on the routes where it is active. Read `components/commandPaletteCommands.ts`. If it builds entries from `ACTION_CATALOG`, nothing more is needed. Otherwise, add an entry there with `actionId: 'global.inbox'`, following the shape of its existing catalog-backed entries. Either way, extend that file's existing test, if it has one, or add one case to `menuActions.test.ts` asserting the palette command list for `routeBase: 'list'` includes `global.inbox`.

- [ ] **Step 2: Widen `isActive`.** Change its first parameter type from `KeyBinding` to `Pick<KeyBinding, 'activation'>`. The body reads only `activation`, so existing callers compile unchanged.

- [ ] **Step 3: Add the pure availability and dispatch functions** in `frontend/src/utils/menuActions.ts`:

```ts
import type { OverlayKind } from '../overlays/OverlayContext';
import { getBindingAction, type KeyActionContext } from './keybindingActions';
import { isActive } from './keybindingActivation';
import { ACTION_CATALOG, type RouteBase } from './keybindings';

/** Exactly the ids the native menu dispatches (services.MenuActionIDs). */
export const MENU_ACTION_IDS = [
  'global.settings',
  'global.inbox',
  'global.search',
  'global.command',
  'global.help',
] as const;

interface MenuActivation {
  routeBase: RouteBase | null;
  activeOverlay: OverlayKind | null;
}

/** Same gate as keys and the palette, so an enabled menu item always acts. */
export function enabledMenuActions(context: MenuActivation): string[] {
  return MENU_ACTION_IDS.filter((actionId) => {
    const definition = ACTION_CATALOG.find((entry) => entry.actionId === actionId);
    return definition !== undefined && isActive(definition, context);
  });
}

/** Re-checks activation at click time; a sign-out race must not run the action. */
export function runMenuAction(actionId: string, context: KeyActionContext): boolean {
  if (!enabledMenuActions(context).includes(actionId)) return false;
  const action = getBindingAction(actionId);
  if (action === undefined) return false;
  action(context);
  return true;
}
```

  Adjust the import paths for `OverlayKind` and `RouteBase` to wherever they are actually exported. Check with the LSP or a search. Don't guess.

- [ ] **Step 4: Report to the engine** in `frontend/src/services/menu.ts`. Use the generated `Shell.SetMenuActions` through `call()` from `services/engine/call.ts`; it's native UI state, not an external write. Mirror an existing service file such as `services/editor.ts` for imports and the `Result` style:

```ts
export function reportMenuActions(enabled: readonly string[]): ResultAsync<void, Fault> {
  return call(() => Shell.SetMenuActions({ enabled: [...enabled] }));
}
```

  Match the exact `call` signature in `call.ts`, and the generated request type name in `frontend/bindings/`.

- [ ] **Step 5: Write the hook** `frontend/src/hooks/useMenuActions.ts`. It holds the React effects only:
  - One effect is keyed on `enabledMenuActions({ routeBase, activeOverlay }).join(',')`. Memoize that string from `activation.routeBase` and `activation.activeOverlay`. The effect calls `reportMenuActions(ids)`. On error, log it once with the project logger at the display or log boundary; don't swallow it.
  - Another effect is keyed on `menuActionRequests`, read via `useStore(appStore, (s) => s.menuActionRequests)`. When the queue is non-empty, it first clears it (`appStore.setState((state) => ({ ...state, menuActionRequests: [] }))`), then runs `runMenuAction(request.actionId, makeCtx())` for each queued request in order. Each one that returns false gets a dev-only warning naming the action ID. Clearing before running means a handler that triggers another render never runs the same request twice.
  - Call `useMenuActions(activation, makeCtx)` inside `useAppHotkeys`, right after `useKeyActionContext`. AGENTS.md keeps key-adjacent dispatch there, and this reuses the same ref-backed context.

- [ ] **Step 6: Write the behaviour tests** in `frontend/src/utils/menuActions.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { enabledMenuActions } from './menuActions';

describe('menu action availability', () => {
  it('enables only actions the current route activates', () => {
    expect(enabledMenuActions({ routeBase: 'list', activeOverlay: null })).toEqual([
      'global.settings', 'global.inbox', 'global.search', 'global.command', 'global.help',
    ]);
    expect(enabledMenuActions({ routeBase: 'diff', activeOverlay: null })).toEqual([
      'global.inbox', 'global.command', 'global.help',
    ]);
    expect(enabledMenuActions({ routeBase: 'settings', activeOverlay: null })).toEqual(['global.inbox']);
  });

  it('disables every menu action while signed out', () => {
    expect(enabledMenuActions({ routeBase: 'login', activeOverlay: null })).toEqual([]);
  });
});
```

  Add one `runMenuAction` case: on the `login` route it returns `false` and never calls the handler. Build the `KeyActionContext` the way `hooks/useAppHotkeys.test.ts` does.

  The expected arrays come from `keybindings.ts:97-145` plus the new `global.inbox` activation. If the catalog's activation differs, fix the expectations to match the catalog, and say so in the report.

- [ ] **Step 7: Verify.** Run `cd frontend && pnpm type-check && pnpm lint && pnpm test`, then the Go checks. Expected: all pass.

- [ ] **Step 8: Commit.**

```bash
git add frontend/src
git commit -m "feat: enable and dispatch native menu actions through the keymap"
```

---

### Task 7: Documentation

**Files:**
- Modify:
  - `AGENTS.md`;
  - `DEVELOPMENT.md`;
  - one page under `website/src/pages/docs/`. Pick the page covering the app's general use or installation, and read the directory first.

- [ ] **Step 1: Update `AGENTS.md`** (`CLAUDE.md` is a symlink, so don't edit it).
  - **Architecture:** add one paragraph covering:
    - `internal/tray`: the pure model plus the Wails adapter;
    - `background.go` / `appmenu.go` in `package main`;
    - the engine's `syncStatusTracker` behind the emitter;
    - the `sync:status` and `menu:action` events;
    - `Shell.SetMenuActions`;
    - the fact that background mode is on only when the tray is ready, which excludes Linux and smoke.
  - **Conventions:** native menu items dispatch by `actionId`, with no accelerators, and the renderer reports their enablement from `isActive`. The dock service is never called on the main thread.
- [ ] **Step 2: Update `DEVELOPMENT.md`.** When a tray is available (macOS, Windows), closing the window keeps Bistro running in the tray, and ⌘Q, the menu's Quit or the tray's Quit exits. On Linux, and under `--smoke`, closing the window quits.
- [ ] **Step 3: Update the user docs page.** Add one short section covering:
  - the tray icon's states (use the exact status lines from the Global Constraints);
  - the tray menu;
  - that closing the window keeps syncing;
  - that on Linux, closing the window quits.
- [ ] **Step 4: Verify** that all the links resolve and that `pnpm` checks for `website/` pass, if it has a lint or type-check script (read `website/package.json`).
- [ ] **Step 5: Commit.**

```bash
git add AGENTS.md DEVELOPMENT.md website/src/pages/docs
git commit -m "docs: describe tray, background mode and app menu"
```

## Manual verification (after Task 7, macOS)

1. **Tray icon:** it appears, and its tooltip and status line track each state. Get each state by signing out, using Sync Now, and turning auto-sync off in Settings. All four icon states (idle, syncing, attention, signed out) must be distinguishable from each other in **both** light and dark menu bars. If any pair isn't, Ruling 5 fails: add monochrome dark variants through `SetDarkModeIcon` before this ships.
2. **Hiding:** closing the window hides it and the Dock icon.
3. **Restoring:** the tray's Show Bistro, a Dock click (when the Dock icon has reappeared) and a second launch each restore the window, with the Dock icon.
4. **Quitting:** ⌘Q and the tray's Quit exit cleanly while the window is hidden.
5. **Menu:** each item works, and the action items are disabled on routes where their keys are inactive.
6. **Appearance:** View ▸ Theme and Compact Density change the app, and stay checked in sync after a change made from Settings.

Windows and Linux need the same checklist before the first release that ships this; see the spec's testing section.
