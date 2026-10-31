# System tray, background mode and app menu: design

Date: 2026-10-31. Status: approved in chat; awaiting written-spec review.
Branch: `experiment/wails3-migration`. Wails v3.0.0 (read the version from `go.mod`).

## Goal

When a tray is available, Bistro keeps syncing after its window is closed. When no tray is available, closing the window quits, exactly as it does today. A tray icon on macOS, Windows and Linux shows the sync engine's state and offers quick actions. The native app menu exposes the main app actions without bypassing the user's keymap.

## Non-goals

- A popover or attached tray window (Wails `AttachWindow`).
- Launch at login.
- Dock or taskbar badges (`dock.SetBadge`).
- Native shortcuts for menu items that dispatch renderer actions.
- Per-repository status in the tray.

## Principles

1. **Use only Wails built-ins and no OS-specific code of our own.**
   - The tray uses `app.SystemTray.New()` with `SetIcon`, `SetDarkModeIcon`, `SetTooltip`, `SetMenu` and `OnClick`. All five are implemented on darwin, windows and linux.
   - Do not use `SetLabel` or `SetTemplateIcon`: text beside the icon is a macOS convention, and `SetTemplateIcon` is barely more than a stub on Windows and Linux.
   - Dock visibility uses the Wails service `github.com/wailsapp/wails/v3/pkg/services/dock`. On macOS, `HideAppIcon` and `ShowAppIcon` switch the activation policy. On Windows and Linux they are no-ops, which is enough there because a hidden window already drops out of the taskbar.
   - Bistro makes the same calls on every platform.
2. **Logic lives in pure Go and the Wails layer is thin.** The tray's content is computed by a pure function that does not import Wails. The adapter only applies the result.
3. **Keys stay renderer-owned.** Menu items that map to an `ACTION_CATALOG` action dispatch it by `actionId` through the same path as the command palette. They get no native shortcut, because a native shortcut would take the key before the WebView, ignore the user's style and overrides, and fire twice. Built-in Wails roles keep their native shortcuts.

## Components

### 1. Engine sync status tracker (`internal/engine/syncstatus.go`)

The tracker owns one `services.SyncStatus` value:

```go
type SyncStatus struct {
    State        string  `json:"state"`          // one of the SyncState* constants
    LastSyncedAt *string `json:"last_synced_at"` // RFC 3339, nil before the first success
    ResumeAt     *string `json:"resume_at"`      // RFC 3339, set only for rate_limited
    ErrorCode    string  `json:"error_code,omitempty"`
}
```

The states are `signed_out`, `syncing`, `up_to_date`, `paused`, `rate_limited` and `error`. They are named constants in `internal/services`.

The rules:

- `syncWatchedRepos` and `syncSingleRepo` call `begin()` when they start and `end(result)` when they finish. A counter of in-flight syncs keeps the state at `syncing` until the last one finishes.
- When the last sync finishes, the state is set by the most severe outcome since the batch began. The order is rate-limited, then error, then success. A success sets `LastSyncedAt`.
- The rate-limited outcome sets `ResumeAt = now + reset_in`, using the same value that `sync:rate-limited` carries. `ResumeAt` is the time after which a retry is allowed, not a promise that a sync is scheduled then. When `ResumeAt` passes, a timer in the tracker recomputes the state from the baseline (`paused` if auto-sync is off, otherwise `up_to_date`, or `error` if the last batch failed for another reason) and emits it. A later `begin()` cancels the timer.
- The state is `signed_out` when there is no authenticated provider. Auth state changes recompute it.
- The state is `paused` when auto-sync is off and nothing is in flight. `ApplySyncSettings` recomputes it.
- Every change is emitted as the new typed event `sync:status`, registered in `internal/services/events.go`. It is also delivered to an engine-side listener: `App.OnSyncStatus(func(SyncStatus))`, which is set before `Start`.
- A new bound method `Sync.Status(ctx) (SyncStatus, error)` returns the current value. It serves the renderer and the tray's first paint.

The tracker never blocks sync. Listener calls run outside its mutex.

The existing events `sync:complete`, `sync:error` and `sync:rate-limited` are unchanged.

### 2. Tray model (`internal/shell/traymodel.go`, pure)

```go
type TrayView struct {
    Icon       TrayIcon     // idle | syncing | attention | signed_out
    Tooltip    string       // "Bistro — <StatusLine>"
    StatusLine string
    Items      []TrayItem   // ordered menu entries
}
type TrayItem struct {
    ID      TrayItemID      // show | sync_now | check_updates | open_logs | quit | separator | status
    Label   string
    Enabled bool
}
func BuildTrayView(status services.SyncStatus, opts TrayViewOptions) TrayView
```

`TrayViewOptions` carries:

- `IsDevBuild bool`, which disables Check for Updates.
- `Now time.Time`.
- `Location *time.Location`, used to format `HH:MM`.

Each state maps to a status line and an icon:

| State | Status line | Icon |
|---|---|---|
| signed_out | `Signed out` | signed_out |
| syncing | `Syncing…` | syncing |
| up_to_date | `Up to date · synced HH:MM` (or `Up to date` before the first sync) | idle |
| paused | `Paused (auto-sync off)` | idle |
| rate_limited | `Rate limited · retry after HH:MM` | attention |
| error | `Sync error (<error_code>)` | attention |

The menu items, in order:

1. The status line, shown disabled.
2. A separator.
3. `Show Bistro`.
4. `Sync Now`. It is disabled when the state is `signed_out` or `syncing`.
5. `Check for Updates…`. It is disabled when `IsDevBuild` is set.
6. `Open Logs`.
7. A separator.
8. `Quit Bistro`.

### 3. Tray adapter and background mode (`internal/shell/tray.go`, `main.go` wiring)

**Tray.**

- `NewTray(app, window, deps)` creates the tray. Its icon assets are PNGs embedded from `build/tray/`. Each `TrayIcon` kind has a light and a dark variant, applied with `SetIcon` and `SetDarkModeIcon`.
- A left click shows the window. Right-click opens the menu, through the Wails default.
- `Apply(TrayView)` rebuilds the menu and sets the icon and tooltip. All UI work goes through Wails' main-thread dispatch.
- The engine's `OnSyncStatus` listener and a one-minute ticker both call `BuildTrayView` and then `Apply`. The ticker exists so that relative times stay correct.

**Tray item actions.**

- `show` shows the window. See "Show" below.
- `sync_now` calls the existing `Sync.Force` path.
- `check_updates` triggers the existing updater check with its own UI.
- `open_logs` uses the existing `Shell` open-logs path.
- `quit` calls `app.Quit()`, which goes through the existing `quitCoordinator`.

**Background mode.** A pure decision function, `closeAction(isTrayReady, isSmoke bool) CloseAction`, returns either `hide` or `quit`. It returns `hide` only when the tray is ready and the app is not in smoke mode.

- On `hide`, the `events.Common.WindowClosing` hook calls `e.Cancel()`, `window.Hide()` and `dock.HideAppIcon()`.
- On `quit`, the hook does not intercept the close.
- The existing window-bounds save keeps working when the window hides.

**Show.** Show calls `dock.ShowAppIcon()`, then `window.Show()`, then `window.Focus()`. It is triggered from:

- the tray;
- `events.Mac.ApplicationShouldHandleReopen`;
- Wails single-instance second launch, which already focuses the window.

**Quit.** Quit happens only through ⌘Q / the App-menu Quit role, the tray's Quit, and OS shutdown. Shutdown order and teardown are unchanged.

**Tray readiness.**

- `isTrayReady` is false if creating the tray fails or reports an error. In that case one warning is logged, and closing the window quits, as it does today.
- Plan task 1 must verify how Wails v3.0.0 reports a failure to register a Linux StatusNotifierItem. If Wails exposes no such signal, `isTrayReady` is false on Linux until a signal exists, so closing the window quits there.

**Smoke.** `--smoke` creates no tray and installs no close hook.

### 4. App menu (`internal/shell/appmenu.go`, built in `main.go`'s `setMenu`)

Menu items marked `action <id>` first show the window (see "Show" in component 3) and then emit the new typed event `menu:action {action_id}`. Every action the menu uses is a `global.*` action, and these are active on every signed-in route. The menu is rebuilt on `OnSyncStatus`: while the state is `signed_out`, every `action` item is disabled, because the login route doesn't activate them. That means an enabled menu item always does something. Items marked `role` are built-in Wails roles. Items marked `native` are handled in Go.

- **Bistro** (macOS App menu; on other platforms these items go in File):
  - About (role);
  - Settings… (action `global.settings`);
  - Check for Updates… (native; hidden in dev builds);
  - separator;
  - Hide, Hide Others, Show All (roles, macOS);
  - separator;
  - Quit (role).
- **File:**
  - Sync Now (native, `Sync.Force`);
  - Inbox (action `global.inbox`);
  - Search (action `global.search`);
  - Command Palette (action `global.command`).
- **Edit:** the existing Edit role.
- **View:**
  - Theme ▸ Light / Dark (native radio items that write `appearance.theme` through the Go settings store);
  - Compact Density (native checkbox that writes `appearance.highDensity`);
  - separator;
  - Reload, Reset Zoom, Zoom In, Zoom Out, Toggle Full Screen (roles).

  The renderer updates through the existing `settings:changed` event. The menu's checked state follows settings changes, including external edits to the settings file.
- **Window:** the Window role.
- **Help:**
  - Keyboard Shortcuts (action `global.help`);
  - Documentation (native, opens `https://get-bistro.com/docs`);
  - Report an Issue (native, opens `https://github.com/Thourum/Bistro/issues/new`);
  - Open Logs (native).

  Both URLs go through the existing validated external-URL path in `services.Shell`. Each is a named constant.

### 5. Renderer

- Add a palette-only action, `global.inbox` ("Go to Inbox"), to `ACTION_CATALOG`. It has no key in any style, and its handler navigates to the inbox route.
- Add handlers for the new events to the exhaustive table in `frontend/src/services/events/index.ts`:
  - `menu:action` → `services/events/menu.ts`. It resolves `getBindingAction(action_id)` and runs it only if the action is active on the current effective route, using the same `isActive` / ref-backed action context as the palette (`useKeyActionContext`). An unknown or inactive id is ignored, and a warning is logged in dev. Given the menu's enablement rule, this happens only as a race, for example a click at the moment of sign-out.
  - `sync:status` → `services/events/sync.ts`. It stores the status in the app store for later UI use. No new UI is in scope.
- After the Go changes, regenerate the bindings with `wails3 task generate` and commit `frontend/bindings/`.

## Error handling

| Failure | Behaviour |
|---|---|
| Tray creation fails, or no Linux tray host is detected | Log one warning. `isTrayReady` is false, and closing the window quits. |
| Sync Now fails | No dialog. The tracker moves to `error` and the status line shows the code. |
| Opening a URL or the logs fails | Logged at warn by the existing `Shell` paths. Never swallowed or shown twice. |
| `menu:action` is unknown or inactive | Ignored, with a warning in dev. |
| The tracker listener panics | Recovered and reported by the existing panic reporter. Sync continues. |

## Testing

These are behaviour tests only, following AGENTS.md and the user's test policy.

1. `traymodel` table test. For each state it checks the status line, the icon, and which items are enabled. This includes Sync Now disabled while signed out or syncing, and Check for Updates disabled in dev.
2. Engine tracker test. Two overlapping repo syncs, one of which fails, must stay `syncing` until both finish and then end in `error`. A rate-limited result must give `resume_at = now + reset_in`.
3. `closeAction` table test covering three cases: tray ready (hide), tray not ready (quit), and smoke (quit, no hook).
4. Renderer service test. `menu:action` dispatches through `getBindingAction` and is refused when the action is inactive.
5. Manual check on macOS:
   - the tray icon in light and dark mode;
   - each state's text;
   - closing the window hides it and the Dock icon;
   - the Dock, the tray and a second launch each bring it back;
   - ⌘Q quits cleanly while the window is hidden;
   - every menu item works.

6. The same manual checklist on Windows and on Linux (a desktop with a StatusNotifierItem host, plus one without) before the first release that ships this. CI smoke runs on Windows and Linux create no tray and install no close hook, so CI proves only that the app builds and starts. It does not validate the tray or background journeys, and they stay unvalidated on those platforms until someone runs the manual checklist.

## Docs

- Update AGENTS.md:
  - **Architecture:** add the tray, background mode, `sync:status` and `menu:action`.
  - **Conventions:** menu items dispatch by `actionId`, with no native shortcuts.
- Update DEVELOPMENT.md: note that when a tray is available, closing the window keeps the app running and ⌘Q or the tray's Quit exits; without a tray (for example, a Linux desktop with no StatusNotifierItem host), closing the window quits.
- Update the user docs under `website/src/pages/docs/`, adding one short section on the tray and background behaviour, including the no-tray case where closing the window quits.
