# Tasks 11–12: Shell service, telemetry, updater

Read first: [README](README.md), [inventory-engine.md](inventory-engine.md) (§ Events, § Lifecycle), [inventory-wails.md](inventory-wails.md). For Tasks 10–12, the ported Electron sources are under `electron/src/main/` and `electron/src/shared/` until Task 13 deletes them. **Read those sources before porting.**

### Task 11: Editor launch and the `Shell` service

**Files:**
- Create: `internal/shell/editor.go`, `editor_unix.go` (`//go:build !windows`), `editor_windows.go`, `internal/shell/platform.go`, `internal/services/shell.go`
- Test: `internal/shell/editor_test.go`, `internal/services/shell_test.go`

**Interfaces:**
- Consumes: `shell.SettingsStore` (Task 10).
- Produces:
  - `type OpenInEditorRequest struct{ RepoRoot string \`json:"repoRoot"\`; RelativePath string \`json:"relativePath"\`; Line int \`json:"line,omitempty"\` }`
  - `func OpenInEditor(req OpenInEditorRequest, editorEnv string) error`
  - `func ParseEditorCommand(s string) ([]string, error)`
  - `func BuildEditorArgs(editor []string, file string, line int) []string`
  - `func ResolveEditorTarget(repoRoot, relativePath string) (string, error)`
  - `type Platform interface{ PickFolder(title string) (string, error); OpenURL(url string) error; OpenPath(path string) error; Quit() }`
  - `func NewPlatform() Platform`, which resolves `application.Get()` at call time
  - `services.Shell` with:
    - `GetSettings(ctx) (shell.Settings, error)`, `SetSettings(ctx, shell.Settings) (shell.Settings, error)`, `ResetSettings(ctx) (shell.Settings, error)`, `OpenSettingsFile(ctx) error`
    - `PickFolder(ctx) (string, error)`, `OpenExternal(ctx, url string) error`, `OpenInEditor(ctx, shell.OpenInEditorRequest) error`
    - `Quit(ctx) error`, `AppInfo(ctx) (AppInfo, error)`, `OpenLogFolder(ctx) error`
  - `AppInfo{Version string \`json:"version"\`; Platform string \`json:"platform"\`}` (platform is `runtime.GOOS`)
  - `Set.Shell`

- [ ] **Step 1: Port the editor modules.** Port `editorRequest.ts`, `editorTarget.ts` and `shared/editorArgs.ts` into `editor.go`, keeping the same rules and messages (listed in the Electron tests).
  - Validation: `repoRoot` absolute, non-empty, no NUL; `relativePath` non-empty, no NUL; `line` 0 (absent) or ≥ 1.
  - Target: `filepath.EvalSymlinks(root)`, require `root/.git` to exist, `EvalSymlinks(filepath.Join(root, rel))`, then contain check `target == root || strings.HasPrefix(target, root+string(filepath.Separator))`.
  - Tokenising: whitespace splits; single quotes are literal; inside double quotes only `\"` and `\\` escape; an unquoted `\` escapes the next character. Empty input → error `$EDITOR is not set`.
  - Args: the base name is lowercased with `.exe`, `.cmd` and `.bat` stripped. `code`, `code-insiders` and `cursor` get `-g file[:line]`; `vim`, `nvim` and `vi` get `+line file`; anything else gets `file`. Leading args from `$EDITOR` are kept.
  - Launch: `exec.Command(cmd, args...)` with no shell and stdio nil; `detach(c)` (unix: `SysProcAttr{Setpgid: true}`; windows: `CREATE_NEW_PROCESS_GROUP`); `c.Start()`, then `c.Process.Release()`.
  - Errors: validation and containment → `ErrEditorRequest` (wrap a sentinel). A missing `$EDITOR` → `ErrEditorNotSet`. A start failure → `ErrEditorLaunch`.
- [ ] **Step 2: Platform.** `platform.go` implements `Platform` over `application.Get()`:
  - `PickFolder` uses `Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).SetTitle(title).PromptForSingleSelection()`.
  - `OpenURL` uses `Browser.OpenURL`.
  - `OpenPath` uses the OS opener via `exec.Command`: `open` on darwin, `xdg-open` on linux, `rundll32 url.dll,FileProtocolHandler` on windows. It is used for the settings file and the log folder.
  - `Quit` uses `Quit()`.
- [ ] **Step 3: `services/shell.go`.**
  - The struct has `Settings *shell.SettingsStore; Platform shell.Platform; EditorEnv func() string; Version string; LogDir string; Panics PanicReporter`.
  - Every method uses `guard` or `guardErr`.
  - `OpenExternal` accepts only `http` and `https` with a host (`url.Parse`); anything else → `CodeInvalid`.
  - `OpenInEditor` maps `ErrEditorNotSet` → `WithReason(CodeInvalid, "editor_not_set", "Set $EDITOR")`, `ErrEditorRequest` → `CodeInvalid`, and `ErrEditorLaunch` → `WithReason(CodeInternal, "editor_launch_failed", …)`.
  - `PickFolder` returns `""` when cancelled.
  - `OpenSettingsFile` makes sure the file exists (write the current settings if it is missing), then calls `OpenPath`.
- [ ] **Step 4: Tests.**
  - `editor_test.go`, as table tests porting every case in `editorRequest.test.ts`, `editorTarget.test.ts` (a temp fixture with `repo/.git/`, `repo/src/app.ts`, `outside/secret.env`; symlink escapes; the sibling `repo-evil` prefix trap; a non-git root; relative and missing roots and targets) and `editorArgs.test.ts`.
  - Launch tests call the unexported `launch(cmd string, args []string) error`. With `exec.LookPath("go")` and `[]string{"version"}` it returns nil. A binary path that does not exist returns `ErrEditorLaunch`.
  - `shell_test.go`: an `OpenExternal` table where `https://x` and `http://x` pass, and `javascript:alert(1)`, `file:///etc/passwd` and `https://` (no host) give `CodeInvalid`. Use a fake `Platform` that records the URL.
- [ ] **Step 5: Wire it.** Add `Shell *Shell` to `Set`, but **do not** construct it in `engine.Services()`, because the engine has no shell. `main.go` (Task 14) sets `set.Shell = &services.Shell{…}`.
- [ ] **Step 6: Verify.** `go vet ./... && go test -race ./internal/shell/... ./internal/services/...`
- [ ] **Step 7: Commit.** `git add -A && git commit -m "feat: add Shell service with settings, dialogs and editor launch"`

### Task 12: Telemetry and updater

**Files:**
- Create: `internal/shell/telemetry.go`, `internal/shell/sentry.go`, `internal/shell/updater.go`, `internal/shell/updater.css`
- Test: `internal/shell/telemetry_test.go`, `internal/shell/sentry_test.go`
- Modify: `go.mod` (`github.com/getsentry/sentry-go`, `github.com/posthog/posthog-go`; latest versions older than 7 days)

**Interfaces:**
- Produces:
  - `func InitSentry(dsn, version string, consent func() bool) (flush func(), err error)`
  - `type SentryReporter struct{ Log *slog.Logger }`, which implements `services.PanicReporter`
  - `func NewTelemetry(dataDir string, log *slog.Logger) *Telemetry`, with `SetEnabled(bool)`, `TrackAppStarted(version string)`, `TrackSettingsChanged(prev, next Settings)` and `Close()`
  - `func StartUpdater(ctx context.Context, app *application.App, version string, log *slog.Logger) error`

- [ ] **Step 1: Check the APIs.** Use the `docs-lookup` agent or Context7 for the installed `sentry-go` (`ClientOptions.BeforeSend`, `Hub.Recover`, `Flush`) and `posthog-go` (`NewWithConfig`, `Config{Endpoint}`, `Enqueue(posthog.Capture{…})`, `Close`). Report it if the docs could not be retrieved.
- [ ] **Step 2: Sentry.** Port `electron/src/main/sentry.ts`.
  - The DSN is the `dsn` argument (ldflags), overridden by `SENTRY_DSN`. Release `bistro@<version>`. Environment from `SENTRY_ENVIRONMENT`, else `production` when the version is not `dev`, else `development`. `TracesSampleRate` 0.
  - `BeforeSend`: return nil unless `consent()`. Otherwise strip user, request, breadcrumbs, contexts and extra; set message `[redacted]`, each exception's `Value` `[redacted]` and `Type` `Error`; reduce stack frame filenames and abs paths to their base names.
  - `SentryReporter.ReportPanic` logs, then calls `sentry.CurrentHub().Recover(recovered)`.
- [ ] **Step 3: PostHog.** Port `electron/src/main/telemetry.ts`.
  - Key from `BISTRO_POSTHOG_API_KEY`, then `POSTHOG_API_KEY`; no key means a no-op.
  - Host from `BISTRO_TELEMETRY_ENDPOINT`, then `BISTRO_POSTHOG_HOST`, then `POSTHOG_HOST`, else `https://us.i.posthog.com`. `resolveTelemetryEndpoint` accepts https, or http only on a loopback host, and rejects URLs with credentials.
  - The distinct id is a UUID persisted at `<dataDir>/telemetry/anonymous-install-id`. Every event carries `telemetrySchemaVersion: 1`. Nothing is enqueued while disabled.
  - `TrackAppStarted` sends `app_started{platform, appVersion}`, where appVersion must match `^[0-9A-Za-z.+-]{1,32}$` (otherwise it is omitted).
  - `TrackSettingsChanged` diffs prev and next per section and sends one `settings_updated{section}` per changed section. The section map is: `appearance`→`appearance`, `keybindings`→`keyboard`, `prList`→`prList`, `navigation`→`navigation` (plus `style`), `privacy`→`privacy` (plus `telemetryEnabled`), `sync`→`sync`, and everything else → `advanced`. `window` changes are ignored.
- [ ] **Step 4: Updater.**
  ```go
  // StartUpdater checks GitHub releases at launch and every 6h. Dev and
  // untagged builds (version "dev" or "") never update.
  func StartUpdater(ctx context.Context, app *application.App, version string, log *slog.Logger) error {
  	if version == "" || version == "dev" {
  		return nil
  	}
  	gh, err := github.New(github.Config{Repository: "Thourum/Bistro", Prerelease: true, ChecksumAsset: "SHA256SUMS"})
  	if err != nil {
  		return err
  	}
  	if err := app.Updater.Init(updater.Config{
  		CurrentVersion: version,
  		Providers:      []updater.Provider{gh},
  		CheckInterval:  6 * time.Hour,
  		Window:         &updater.BuiltinWindow{CSS: updaterCSS},
  	}); err != nil {
  		return err
  	}
  	go func() {
  		if err := app.Updater.CheckAndInstall(ctx); err != nil {
  			log.Warn("update check failed", "err", err)
  		}
  	}()
  	return nil
  }
  ```
  - `//go:embed updater.css` → `updaterCSS`. Read `examples/updater/main.go` in the Wails module cache and confirm the field names and that `*updater.BuiltinWindow` satisfies `WindowOption`. Adjust to what the source says.
  - `updater.css` sets the 18 variables from [inventory-wails.md](inventory-wails.md), using the light and dark colour, radius, spacing and font tokens from `docs/DESIGN.md`. **Copy the token values from DESIGN.md**; do not invent any.
- [ ] **Step 5: Tests.**
  - `telemetry_test.go`:
    - a `resolveTelemetryEndpoint` table: https ok, `http://127.0.0.1:9` ok, `http://example.com` rejected, `https://u:p@x` rejected
    - `TrackSettingsChanged` with navigation and window changed → exactly one event, `settings_updated{section:"navigation", style}`, through an injectable capture func
    - a disabled client enqueues nothing
  - `sentry_test.go`: `BeforeSend` with consent false → nil; with consent true → message, exception value, user and breadcrumbs redacted, and frame filenames reduced to base names.
  - No updater test: it is configuration only.
- [ ] **Step 6: Verify.** `go vet ./... && go test -race ./internal/shell/...`
- [ ] **Step 7: Commit.** `git add -A && git commit -m "feat: add Sentry, PostHog and GitHub updater to the Go shell"`
