# Wails v3.0.0 API facts used by this plan

Checked in `~/go/pkg/mod/github.com/wailsapp/wails/v3@v3.0.0`. `A` below means `pkg/application`. Check here again before you write against an API; don't rely on memory.

## Install and versions
- The CLI: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0`. Wails' own `go.mod` says `go 1.26.0`.
- npm: `@wailsio/runtime` `3.0.0`, plus the Vite plugin `import wails from "@wailsio/runtime/plugins/vite"`, used as `wails("./bindings")`. The plugin fails the build when bindings are missing.

## Services (A/services.go, A/bindings.go, A/messageprocessor_call.go)
- Registering services:
  - `application.NewService[T any](instance *T) Service`
  - `Options.Services []Service`
  - `func (a *App) RegisterService(service Service)`, which works until `Run()` starts
- `func application.Get() *App` returns the current app.
- Service lifecycle:
  - optional `ServiceStartup(ctx, ServiceOptions) error`, where ctx is the app ctx, cancelled before shutdown
  - optional `ServiceShutdown() error`, run in reverse order
- **Every exported method is bound.**
  - A leading `context.Context` is detected and left out of the JS signature.
  - Each call runs in its own goroutine with ctx `WithCancel`.
  - JS `promise.cancel()` cancels that ctx, and so does closing the window.
  - A cancelled promise rejects with `CancelError`.
- **Errors.**
  - A returned non-nil error rejects with `RuntimeError`.
  - Its `err.cause` is `json.Marshal(&err)` of the concrete error, unless `Options.MarshalError` or a service's own `MarshalError` overrides it. A `*ServiceError` with exported fields therefore arrives as `{code,message,reason}`.
  - Returned errors that are not `ServiceError` arrive as `{}`.
- **Panics.**
  - A panic in a bound method is recovered and becomes a `RuntimeError`.
  - `Options.PanicHandler func(*PanicDetails{StackTrace, Error, Time, FullStackTrace})` is called with it.
  - Our `guard` recovers first, so the renderer sees `internal`.

## Events (A/events.go, A/event_manager.go)
- Registering: `application.RegisterEvent[Data any](name string)` goes in `init()`. It panics on a duplicate or system name.
- Emitting: `app.Event.Emit(name string, data ...any) bool`.
  - With one data argument, that argument is the data.
  - Registered events are **type-checked exactly**: emit `T`, never `*T`. A mismatch goes to `ErrorHandler` and cancels the event.
- Bindings: the generator writes `bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts`, which augments `@wailsio/runtime` `Events.CustomEvents`.
- JS: `Events.On(name, (ev) => ev.data)` returns an unsubscribe function, and `ev.data` is typed.

## Bindings generator
- Flags: `wails3 generate bindings [-d frontend/bindings] [-ts] [-i] [-names] [-clean=true] [-time-type string|Date] [-f buildflags] [pattern=.]`.
- Template task: `wails3 generate bindings -f '{{.BUILD_FLAGS}}' -clean=true -ts -i`. **Use `-ts -i`.**
- Output for `github.com/itterno/bistro/internal/services`:
  - `frontend/bindings/github.com/itterno/bistro/internal/services/{auth,diff,…}.ts` (lowercased type names)
  - `models.ts`
  - `index.ts` (`export { Auth, Diff, … }` plus the model types)
- Each method is generated as `export function Get(req): $CancellablePromise<DiffResponse>`.
- Type mapping:

  | Go | TS |
  |---|---|
  | `time.Time` | `string` |
  | `[]byte` | base64 `string` |
  | `*T` | `T \| null` |
  | maps, slices (with `-i`) | `… \| null` |
  | `any` | `any` |
  | structs | interfaces/types (with `-i`) |

## App options and lifecycle (A/application_options.go, A/application.go)
- `application.New(Options)` enforces `SingleInstance *SingleInstanceOptions{UniqueID string; OnSecondInstanceLaunch func(SecondInstanceData); ExitCode int}` **inside `New`**, so a second instance exits before it does any work. The callback runs on a goroutine in the first instance.
- Shutdown options: `OnShutdown func()`, `PostShutdown func()`, `ShouldQuit func() bool`, plus `app.OnShutdown(f)` and `app.Quit()`.
  - Order on shutdown: the app ctx is cancelled, then `OnShutdown` runs, then services shut down in reverse order, then windows close, then `PostShutdown` runs.
- First page ready: `events.Common.WindowRuntimeReady` fires when the page's runtime reports ready, on every load. Subscribe with `window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent))`.
- `Options.Mac.ApplicationShouldTerminateAfterLastWindowClosed bool`.

## Windows, menus, dialogs, browser
- `app.Window.NewWithOptions(application.WebviewWindowOptions{Name, Title, Width, Height, MinWidth, MinHeight, InitialPosition: application.WindowXY|WindowCentered, X, Y, StartState: application.WindowStateMaximised, URL: "/", Mac: application.MacWindow{TitleBar: application.MacTitleBarHiddenInset, InvisibleTitleBarHeight: 32}})` returns `*WebviewWindow`.
- To persist bounds: listen for `OnWindowEvent(events.Common.WindowDidMove / WindowDidResize, …)`, then read `w.Bounds() Rect{X,Y,Width,Height}` and `w.IsMaximised()`.
- Focusing a window: `w.Show()`, `w.Focus()`, `w.Restore()`.
- Menus: `menu := app.Menu.New()`, `menu.AddRole(application.AppMenu)`, `menu.AddRole(application.EditMenu)`, `app.Menu.SetApplicationMenu(menu)`.
- Pick a directory: `app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).CanCreateDirectories(true).SetTitle(t).PromptForSingleSelection() (string, error)`. Cancel returns `""`.
- Open a URL: `app.Browser.OpenURL(url string) error`.

## Updater (`github.com/wailsapp/wails/v3/pkg/updater`, `…/pkg/updater/providers/github`)
- Config: `updater.Config{CurrentVersion string /* no "v" */; Providers []updater.Provider; CheckInterval time.Duration; Window updater.WindowOption}`. With `PublicKey` nil, digest-only releases install.
- Updater methods:
  - `app.Updater.Init(cfg) error`
  - `app.Updater.CheckAndInstall(ctx) error` (opens the built-in window when an update exists)
  - `app.Updater.StopPeriodicCheck()`
- GitHub provider: `github.New(github.Config{Repository, Prerelease, ChecksumAsset}) (*github.Provider, error)`.
- `DefaultAssetMatcher` picks the first asset whose lowercase name contains GOOS and the arch (aliases x86_64/x64 → amd64, aarch64 → arm64). It skips `-installer.`/`_installer.` assets, signatures and checksum files.
- Archives: `.zip` and `.tar.gz` must contain **exactly one top-level entry** (the `.app` or the binary).
- Theming: `&updater.BuiltinWindow{CSS: css}`. The CSS variables are `--bg --surface --surface-2 --fg --fg-dim --fg-faint --border --accent --accent-fg --accent-dim --success --error --warning --radius --radius-sm --pad --gap --font`.
- Working example: `examples/updater/main.go`. Read it.

## Templates, build, assets
- Generate a scaffold with `wails3 init -n <name> -t react` (the TS template; check flags with `wails3 init -h`). The scaffold includes:
  - a root `Taskfile.yml`
  - `build/Taskfile.yml`
  - `build/config.yml`
  - `build/{darwin,windows,linux}/Taskfile.yml`, plus Info.plist, NSIS and nfpm/AppImage configs
- Assets:
  - `//go:embed all:frontend/dist`
  - `Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)}`
- Dev mode:
  - `wails3 dev -config ./build/config.yml -port <vite port>` sets `FRONTEND_DEVSERVER_URL`.
  - Non-production builds then reverse-proxy to Vite (the template's port is 9245).
- Packaging:
  - Windows NSIS writes `bin/<app>-<arch>-installer.exe`.
  - Linux's default deps are GTK4 + `webkitgtk-6.0`.
  - The minimum macOS is 12.0.
