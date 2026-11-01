# Tasks 9–10: Engine without WS, settings store

Read first: [README](README.md), [inventory-engine.md](inventory-engine.md) (§ Events, § Lifecycle), [inventory-wails.md](inventory-wails.md). For Tasks 10–12, the ported Electron sources are under `electron/src/main/` and `electron/src/shared/` until Task 13 deletes them. **Read those sources before porting.**

### Task 9: Engine without WS

**Files:**
- Modify: `internal/services/events.go` (every Task 9 row of the events table), `internal/engine/bridge.go`, `syncloop.go`, `expertise.go`, `llm.go`, `app.go`, `config.go`
- Create: `internal/engine/supervise.go`, `internal/engine/options.go`
- Modify: `internal/prsync/workerpool/` (panic → task error) and its test
- Test: `internal/engine/bridge_test.go` (rewrite), `internal/engine/supervise_test.go`; update `blockers_test.go` and `syncloop_test.go` to use `servicestest.Recorder`
- Delete:
  - `internal/ws/`, `cmd/bistro-sync/`, `internal/engine/handler/` (whatever is left)
  - `internal/engine/handlers.go`, `wsaddr.go`, `wsaddr_test.go`, `ready.go`, `ready_test.go`, `app_repolocation_test.go`
  - the `gorilla/websocket` and `gofrs/flock` requirements (`go mod tidy`)

**Interfaces:**
- Consumes: `services.Set` with all of Tasks 3–8, and `Emitter`.
- Produces:
  - `func New(opts Options) *App`
  - `type Options struct{ DBPath string; Log *slog.Logger; SyncInterval time.Duration; AutoSync, ReplayQueue bool; AuthCallbackTimeout time.Duration; RulesDir, GitHubClientID, GitHubClientSecret string }`
  - `func (a *App) Start(ctx context.Context) error`: opens the DB, runs migrations, prepares everything, and starts the supervised loops. It does not block.
  - `func (a *App) Stop()`: cancels the loops, waits for them, closes the DB.
  - `func (a *App) Services() *services.Set`
  - `func (a *App) SetEmitter(services.Emitter)`, `func (a *App) SetPanicReporter(services.PanicReporter)` (these must be called before `Start`)
  - `ApplySyncSettings`, `ApplyClassificationSettings` (Tasks 4 and 8)
  - `func supervise(ctx context.Context, name string, rep services.PanicReporter, log *slog.Logger, sleep func(time.Duration), fn func(context.Context))`

- [ ] **Step 1: Declare the remaining events.** In `events.go`, add the `const`, payload struct and `RegisterEvent` line for every row of the [events table](inventory-engine.md#events) marked Task 9, with json tags equal to the old `data` keys. `ByokAuthFailed` is `struct{}`. `PRMetadataChanged.Old`/`New` are `string`: format non-string values with `fmt.Sprint`.
- [ ] **Step 2: Typed bridge.** Replace `wsEventMsg` with a pure mapping function, plus the goroutine that emits:
  ```go
  // bridgeEvent maps an orchestrator event onto a registered Wails event.
  // ok is false for event types the renderer does not consume.
  func bridgeEvent(ev events.Event) (name string, payload any, ok bool) {
  	d := eventData(ev.Data)
  	switch ev.Type {
  	case "rate_limited":
  		return services.EventRateLimited, services.RateLimited{ResetIn: d.float("reset_in")}, true
  	case "PR_METADATA_CHANGED":
  		return services.EventPRMetadataChanged, services.PRMetadataChanged{PRID: d.str("pr_id"), Owner: d.str("owner"), Repo: d.str("repo"), Number: d.int("number"), Field: d.str("field"), Old: d.str("old"), New: d.str("new")}, true
  	case "THREAD_RESOLVED", "THREAD_REOPENED":
  		return services.EventThreadStateChanged, services.ThreadStateChanged{ThreadID: d.str("thread_id"), PRID: d.str("pr_id"), Resolved: ev.Type == "THREAD_RESOLVED"}, true
  	// CI_STATUS_CHANGED, COMMENT_POSTED, DRAFTS_CHANGED, action_complete, pr_approved: same pattern
  	}
  	return "", nil, false
  }
  ```
  - `eventData` is a small unexported accessor over `map[string]any`. `str` uses `fmt.Sprint` for non-strings and `""` for missing keys. `int` accepts `int`, `int64` and `float64`. `float` accepts the numeric kinds.
  - Read each emitter (paths in the inventory) and use its real key types. Cover every case in the comment as well as the three shown.
  - The goroutine calls `a.emitter.Emit(name, payload)` when `ok` is true, runs under `supervise`, and drops the `a.wsServer == nil` guard.
- [ ] **Step 3: Other emitters.**
  - `syncloop.go`: `a.emitter.Emit(services.EventSyncComplete, services.SyncComplete{Scope: …, RepoID: …})` and `services.EventSyncError` with `SyncError{ErrorCode: "SYNC_FAILED", Message: …}`.
  - `expertise.go`: `broadcastExpertiseEvent` switches on the indexer's event type and emits `ExpertiseProgress` / `ExpertiseReady` / `ExpertiseError` built from the indexer payload structs directly. `broadcastExpertiseNudge` emits `ExpertiseNudge`.
  - `llm.go`: `TierConfig.Emit` maps `COMMENTS_READY` → `CommentsClassified{PRID}`, `BYOK_AUTH_FAILED` → `ByokAuthFailed{}`, `BYOK_USAGE` → `ByokUsage{…}`.
  - Afterwards, `grep -rn "Broadcast" internal/` must return nothing.
- [ ] **Step 4: Supervisor.**
  ```go
  // supervise runs fn until ctx is done. A panic is logged and reported, then fn
  // restarts after a backoff (1s doubling, capped at 30s). A normal return ends it.
  func supervise(ctx context.Context, name string, rep services.PanicReporter, log *slog.Logger, sleep func(time.Duration), fn func(context.Context)) {
  	backoff := time.Second
  	for ctx.Err() == nil {
  		if !runRecovered(ctx, name, rep, log, fn) {
  			return
  		}
  		sleep(backoff)
  		backoff = min(backoff*2, 30*time.Second)
  	}
  }

  // runRecovered reports whether fn panicked.
  func runRecovered(ctx context.Context, name string, rep services.PanicReporter, log *slog.Logger, fn func(context.Context)) (panicked bool) {
  	defer func() {
  		if r := recover(); r != nil {
  			panicked = true
  			log.Error("background loop panicked; restarting", "loop", name, "panic", r)
  			if rep != nil {
  				rep.ReportPanic(name, r, debug.Stack())
  			}
  		}
  	}()
  	fn(ctx)
  	return false
  }
  ```
  Production passes a `sleep` that also returns early on `ctx.Done()`: `func(d time.Duration) { select { case <-time.After(d): case <-ctx.Done(): } }`. Run the sync loop, the event bridge and the expertise `RestartInterrupted` goroutine under `supervise`, and track them with a `sync.WaitGroup` that `Stop` waits on.
- [ ] **Step 4b: Workers.** The spec covers "sync loop, workers". Workers are one-shot tasks, so a restart loop is the wrong model; instead, a panic becomes that task's error.
  - In `internal/prsync/workerpool`, wrap each task execution in a `recover` that turns the panic into `fmt.Errorf("worker task panicked: %v", r)`, logs the stack, and **reports** it. To report, add an optional `OnPanic func(recovered any, stack []byte)` to the pool's construction options; `prsync` sets it to call the engine's `services.PanicReporter` (`ReportPanic("workerpool", r, stack)`), and `workerpool` still imports nothing from `services`. The pool keeps running, and the error flows back to the caller like any other task error, so the sync loop reports it as a sync failure.
  - Then audit every other goroutine with `rg -n "go func|go [a-zA-Z.]+\(" internal/ --glob '!*_test.go'`. Each one is either:
    - under `supervise`;
    - recovering into an error;
    - or listed in your report as short-lived and accepted (for example a single `select`-and-return helper).
  - Test: in `workerpool`, one task panics and the others complete. The panicking task's result is an error whose message contains `panicked`, and `OnPanic` was called once.
- [ ] **Step 5: Lifecycle.**
  - Move the flag defaults from `cmd/bistro-sync/main.go` into `Options` and `New(opts)`. Zero values mean the defaults: sync 5 m, callback timeout 5 m. The old setters become `Options` fields; delete setters with no remaining caller.
  - Move the "load the stored token from the keychain and `SetProvider`" step from `cmd/bistro-sync/main.go` into `Start`, after the stores are prepared.
  - `Start` runs the old `Run` steps 1–5, starts the supervised loops, and returns. It does no WS work, no `emitReady` and no signal wait.
  - `Stop` cancels the root ctx, waits for the group, then closes the DB.
  - Delete `wsPort` everywhere, along with the flock: single-instance now comes from Wails `SingleInstance` in `main.go`.
- [ ] **Step 6: Delete.** Remove everything in the Files/Delete list, then run `go mod tidy`. The embedded GitHub client id var moves with `main` to the root `main.go` in Task 14. Until then `Options.GitHubClientID` is set by callers; no var is needed here.
- [ ] **Step 7: Tests.**
  - `bridge_test.go`: a table over `bridgeEvent` with one row per mapped type (input `events.Event` → expected name and payload), plus one row for an unmapped type (`sync_started` → `ok == false`). Delete `TestWsEventMsg_MarshalShape`.
  - `supervise_test.go`:
    - `fn` panics on its first two calls, then blocks until ctx is done. The test asserts that `fn` ran 3 times, that `sleep` received `1s, 2s` in that order, and that the reporter saw 2 panics. Cancel ctx to finish.
    - `fn` returns normally → `supervise` returns after one call.
  - Port `blockers_test.go` and `syncloop_test.go` to `servicestest.Recorder`. `syncloop_test` asserts `sync:complete` payloads.
- [ ] **Step 8: Verify.** Run each and expect it to be empty or pass:
  ```bash
  go vet ./... && go test -race ./...
  grep -rn "internal/ws\|wsServer\|gorilla" --include=*.go . ; echo "expect no output"
  ```
- [ ] **Step 9: Commit.** `git add -A && git commit -m "feat: run the engine in-process with typed Wails events, no WebSocket"`

### Task 10: `shell` settings store

**Files:**
- Create: `internal/shell/settings.go`, `internal/shell/settings_leaves.go`; Test: `internal/shell/settings_test.go`
- Modify: `internal/services/events.go` (add `EventSettingsChanged = "settings:changed"`, payload `shell.Settings`)

**Interfaces:**
- Produces:
  - `type Settings struct`, whose fields and json names are below
  - `func DefaultSettings() Settings`
  - `type SettingsStore struct`
  - `func NewSettingsStore(path string, log *slog.Logger) *SettingsStore`
  - `Load() error`, `Get() Settings`, `Set(next Settings) (Settings, error)`, `Reset() (Settings, error)`, `Path() string`, `SetWindow(WindowBounds) error`
  - `OnChange(func(prev, next Settings))`
  - `Watch(ctx context.Context, every time.Duration)`

- [ ] **Step 1: Types and defaults.** Port them from `electron/src/types/settings.ts`. They must match it **exactly**: same json names and same defaults, because the renderer reads these through the generated types.
  ```go
  type Settings struct {
  	Schema         int                    `json:"$schema"`
  	Appearance     AppearanceSettings     `json:"appearance"`
  	Sync           SyncSettings           `json:"sync"`
  	PRList         PRListSettings         `json:"prList"`
  	Diff           DiffSettings           `json:"diff"`
  	Privacy        PrivacySettings        `json:"privacy"`
  	Repositories   RepositorySettings     `json:"repositories"`
  	Classification ClassificationSettings `json:"classification"`
  	Navigation     NavigationSettings     `json:"navigation"`
  	Keybindings    map[string][]string    `json:"keybindings"`
  	Window         WindowBounds           `json:"window"`
  }
  // WindowBounds is written by the shell on move/resize; zero Width means "centre at default size".
  type WindowBounds struct {
  	X         int  `json:"x"`
  	Y         int  `json:"y"`
  	Width     int  `json:"width"`
  	Height    int  `json:"height"`
  	Maximised bool `json:"maximised"`
  }
  ```
  - Defaults: `$schema` 1. `appearance`: theme `light`, uiFont `"Geist Variable", "Geist", "Inter", system-ui, sans-serif`, codeFont `"JetBrains Mono", "Geist Mono Variable", "Fira Code", "IBM Plex Mono", monospace`, uiFontSize 14, codeFontSize 13, and highDensity / onboardingComplete / tourComplete false. `sync`: autoSync true, syncInterval 60, notifications true. Then `prList.view` `flat`, `diff.style` `unified`, `privacy.telemetryEnabled` false, and `repositories.devFolderPath` null (`*string`). `classification`: mode `rules`, dismissalScope `local`, provider `anthropic`, model and endpoint `""`. `navigation.style` `vim`, `keybindings` `{}`, and `window` zero.
  - **Check each default against the TS file.**
- [ ] **Step 2: Per-leaf normalisation.** This is the port of `mergeSettings` / `normalise`. In `settings_leaves.go`, declare one table entry per leaf: `{section, key, apply func(raw json.RawMessage, s *Settings) bool}`. `apply` decodes and validates, and returns false (leaving `s` untouched) when the value is invalid.
  - Rules: enums must be one of the TS literals; font sizes and `syncInterval` must be finite and `> 0`; strings must be strings; `devFolderPath` must be a string or null.
  - `keybindings` is one leaf, validated whole as `map[string][]string`.
  - `window` is one leaf: its width and height must be 0 or ≥ 400.
  ```go
  // normalise starts from base and applies every valid leaf found in raw.
  // Unknown sections and keys are ignored; $schema is always 1.
  func normalise(raw map[string]json.RawMessage, base Settings) Settings
  ```
  - `Load` reads the file. If it is missing or not a JSON object, it uses defaults; otherwise it runs `normalise(raw, DefaultSettings())`. A section's `json.RawMessage` is itself decoded into `map[string]json.RawMessage` for its leaves.
  - `Set(next)` marshals `next` to the same map shape and runs `normalise(raw, current)`, so an invalid leaf keeps its current value. It then restores `current.Window`: bounds are shell-owned and written only by `SetWindow`, so a renderer snapshot cannot overwrite them.
- [ ] **Step 3: Persistence and change notification.**
  - Writes are atomic: write a temp file in the same dir, then rename. The JSON is pretty-printed with a trailing newline, and `MkdirAll` runs first.
  - `Set` and `Reset` write, record the file's new mtime, and call every `OnChange` listener with `(prev, next)` when they differ.
  - `SetWindow` writes without notifying, because moving the window must not fire `settings:changed`.
  - `Watch` polls `os.Stat` every `every`. An mtime different from the last one recorded means an external edit. Re-read the file and apply `normalise(raw, current)`, **not** defaults, so a half-saved or invalid edit keeps the last valid values. If the file is not a JSON object, keep the current settings, log a warning and don't notify. Otherwise notify on change. It stops on `ctx.Done()`.
- [ ] **Step 4: Register the event.** `RegisterEvent[shell.Settings](EventSettingsChanged)`, with `services` importing `shell`. `shell` must not import `services`.
- [ ] **Step 5: Tests** in `settings_test.go`, all using `t.TempDir()`:
  1. A missing file loads as `DefaultSettings()`.
  2. Given `{"appearance":{"theme":"purple","uiFontSize":16},"bogus":{}}`, theme stays `light`, uiFontSize is 16, and `$schema` is 1.
  3. `Set` with `navigation.style = "nano"` keeps the current style while other changed leaves apply, and the listener fires once with prev/next.
  4. An external write to the file is picked up by `Watch` (10 ms poll): the listener fires and `Get()` reflects it.
  5. `SetWindow` does not fire the listener.
  6. After `SetWindow({X:5,…})`, calling `Set` with a snapshot whose `window` is zero keeps `X == 5`.
  7. An external write of `{not json` keeps the current settings and fires no listener. An external write with `"theme":"purple"` keeps the current theme rather than resetting it to the default.
- [ ] **Step 6: Verify.** `go vet ./... && go test -race ./internal/shell/...`
- [ ] **Step 7: Commit.** `git add -A && git commit -m "feat: add Go settings store for the Wails shell"`

