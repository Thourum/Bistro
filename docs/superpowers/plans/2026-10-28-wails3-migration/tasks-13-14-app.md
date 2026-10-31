# Tasks 13–14: Frontend relocation, Wails entry point

Read first: [README](README.md), [inventory-wails.md](inventory-wails.md), [inventory-renderer.md](inventory-renderer.md).

### Task 13: Move the renderer to `frontend/` and delete `electron/`

**Files:**
- Move (`git mv`):
  - `electron/src/renderer/` → `frontend/src/`
  - `electron/src/shared/benchSafety.ts` → `frontend/src/shared/benchSafety.ts`
  - `electron/src/types/{settings,engine,ipc}.ts` → `frontend/src/types/electron/{settings,engine,ipc}.ts` (temporary; Task 18 deletes them)
  - renderer tests in `electron/tests/` → `frontend/tests/`
  - `electron/scripts/bench-large-diff.mjs` → `frontend/scripts/`
  - `electron/stitch_terminal_authentic_prd/` → `docs/design/stitch_terminal_authentic_prd/`
  - `electron/assets/icon.png` → `build/appicon.png`, `icon.icns` → `build/darwin/icons.icns`, `icon.ico` → `build/windows/icon.ico`, `icon.icon` → `build/darwin/icon.icon`
  - `electron/build/entitlements.mac.plist` → `build/darwin/entitlements.plist`
  - `electron/CLAUDE.md` → `frontend/CLAUDE.md`, with paths fixed
- Create: `frontend/package.json`, `frontend/vite.config.ts`, `frontend/vitest.config.ts`, `frontend/tsconfig.json`, `frontend/biome.json`, `frontend/knip.json`, `frontend/components.json`
- Modify: `pnpm-workspace.yaml`, `pnpm-lock.yaml` (regenerated), `.github/workflows/ci.yml` (the Electron job becomes a frontend job)
- Delete: the rest of `electron/`. That covers `src/main/`, `src/shared/editorArgs*`, `forge.config.ts`, `vite.{main,preload,renderer}.config.ts`, `tsconfig.{main,node,renderer}.json`, `scripts/build-engine.mjs`, `scripts/smoke-macos.sh`, and main-process tests.

**Interfaces:**
- Produces a pnpm package at `frontend/` with the scripts `dev`, `build`, `type-check`, `lint`, `test`, `test:watch`, `bench:large-diff`. `pnpm build` writes `frontend/dist/`. Path aliases are `@renderer/*` → `src/*` and `@shared/*` → `src/shared/*`. The renderer source is unchanged apart from import paths.

- [ ] **Step 1: Sort the tests.** For each file in `electron/tests/**` and `electron/src/**/*.test.ts`, check its imports:
  - imports `src/main/` or `src/shared/editorArgs` → **delete it**, because its logic was ported to Go in Tasks 10–12;
  - imports renderer code → **move it** under `frontend/tests/` (or keep it beside its source under `frontend/src/`).

  `tests/unit/{telemetry,updater,crashReport,settings}.test.ts` are main-process; check `settings-normalisation.test.ts` and `store.test.ts` by reading their imports. List every delete in the commit body.
- [ ] **Step 2: Do the moves** above, then delete the remainder of `electron/`. Keep `electron/node_modules` out of git; it is already untracked, so just `rm -rf electron` after the moves.
- [ ] **Step 3: `frontend/package.json`.**
  - `"name": "bistro-frontend"`, `"private": true`, `"type": "module"`.
  - Dependencies: copy every renderer dependency from the old `electron/package.json` at the **same version ranges**. Drop the main-only packages: `electron-log`, `electron-store`, `execa`, `posthog-node`, `update-electron-app`, `wait-on`, `ws`. `@sentry/electron` stays until Task 18.
  - devDependencies: `@biomejs/biome`, `@types/node`, `@types/react`, `@types/react-dom`, `@vitejs/plugin-react`, `@vitest/coverage-v8`, `@fontsource-variable/geist`, `@fontsource-variable/geist-mono`, `jsdom`, `knip`, `typescript`, `vite`, `vitest`, all at their old ranges. Drop `electron`, `@electron-forge/*`, `@electron/fuses` and `@types/ws`.
  - Scripts: `"dev": "vite"`, `"build": "vite build"`, `"type-check": "tsc -p tsconfig.json --noEmit"`, `"lint": "biome check src"`, `"test": "vitest run"`, `"test:watch": "vitest"`, `"bench:large-diff": "node scripts/bench-large-diff.mjs"`. Fix the script's import path to `../src/utils/diff-parser.ts`.
- [ ] **Step 4: Configs.**
  - **`vite.config.ts`.** Merge from `vite.renderer.config.ts`:
    - `root: 'src'`, `base: './'`;
    - `build.outDir: '../dist'`, `emptyOutDir: true`;
    - plugins: react and tailwind;
    - `worker.format: 'es'`;
    - aliases `@renderer` and `@shared`, plus `dedupe` for react and react-dom;
    - `server: { host: '127.0.0.1', port: Number(process.env.WAILS_VITE_PORT) || 9245, strictPort: true }`;
    - `define`: `import.meta.env.VITE_APP_VERSION` = `JSON.stringify(process.env.BISTRO_VERSION ?? 'dev')`, and keep `VITE_SENTRY_ENABLED` as it was.
  - **`tsconfig.json`:** the old base strict options plus the renderer options (`lib` DOM, `jsx: react-jsx`, `types: ["vite/client"]`). Include `src`, `tests` and `vite.config.ts`. Paths are `@renderer/*` and `@shared/*`.
  - **`vitest.config.ts`:** the old config, with the `@shared`/`@renderer` aliases re-pointed, includes `tests/**` and `src/**/*.{test,spec}.{ts,tsx}`, and coverage thresholds dropped (they covered `src/main`).
  - **`biome.json`:** copy it, with `vcs.root` `..`.
  - **`knip.json`:** entries `src/main.tsx`, `vite.config.ts`, `vitest.config.ts`, `scripts/bench-large-diff.mjs`.
  - **`components.json`:** css path `src/styles/globals.css`.
- [ ] **Step 5: Workspace and install.**
  - `pnpm-workspace.yaml`: `packages: [frontend/, website/]`.
  - Drop the `allowBuilds` entries whose packages no longer appear in the lockfile after install: `@sentry/node-native-stacktrace`, `fs-xattr`, `macos-alias`. Check them against `pnpm why <pkg>`.
  - Run `cd frontend && pnpm install`. `minimumReleaseAge` applies; never bypass it.
- [ ] **Step 6: CI.** Rename the `electron` job to `frontend` ("Frontend typecheck, lint, and tests"), with `working-directory: frontend`. Its steps stay the same.
- [ ] **Step 7: Verify.**
  ```bash
  cd frontend && pnpm type-check && pnpm lint && pnpm test && pnpm build && ls dist/index.html
  cd .. && go vet ./... && go test -race ./...
  ```
  Expected: every command passes. Renderer behaviour is unchanged: it still imports `window.electron` and `wsClient`, and both still type-check.
- [ ] **Step 8: Commit.** `git add -A && git commit -m "refactor: move renderer to frontend/ and remove the Electron shell"`

### Task 14: Wails entry point, Taskfile, `build/`, bindings, `--smoke`

**Files:**
- Create: `main.go`, `window.go`, `Taskfile.yml`, `build/config.yml`, `build/Taskfile.yml`, `build/{darwin,windows,linux}/…` (from the template), and `frontend/bindings/**` (generated, committed)
- Modify: `frontend/package.json` (add `@wailsio/runtime` `3.0.0`), `frontend/vite.config.ts` (the `wails` plugin and a `@bindings` alias), `frontend/tsconfig.json` and `frontend/vitest.config.ts` (`@bindings/*` alias), `internal/logging` (receives `setupLogging` if Task 9 left it elsewhere), `.githooks/pre-commit` (only if Step 6 needs it)

**Interfaces:**
- Consumes:
  - `engine.New(engine.Options)`, `Start`, `Stop`, `Services`, `SetEmitter`, `SetPanicReporter`, `ApplySyncSettings`, `ApplyClassificationSettings` (Task 9)
  - `shell.NewSettingsStore`, `shell.NewPlatform`, `shell.InitSentry`, `shell.SentryReporter`, `shell.NewTelemetry`, `shell.StartUpdater`, `services.Shell` (Tasks 10–12)
- Produces:
  - the `bistro` binary, with `--smoke`, `-ldflags -X main.version`, `-X main.embeddedGitHubClientID` and `-X main.sentryDSN`;
  - `frontend/bindings/github.com/itterno/bistro/internal/services/index.ts`, which exports every service namespace, reached through the alias `@bindings/services`;
  - the event typings in `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts`;
  - Taskfile tasks `dev`, `generate`, `build`, `package:darwin`, `package:windows`, `package:linux`.

- [ ] **Step 1: Install the CLI and scaffold.**
  ```bash
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
  wails3 init -h   # confirm flags
  wails3 init -n bistro -t react -d /tmp/wails-tmpl
  ```
  Copy `/tmp/wails-tmpl/build/` into `build/` **without overwriting** the icons and entitlements Task 13 moved there. Copy `/tmp/wails-tmpl/Taskfile.yml` too. Adapt them:
  - `APP_NAME: bistro`, product name `Bistro`, bundle id `app.bistro.review`, `PACKAGE_MANAGER: pnpm`, frontend dir `frontend`;
  - `build/config.yml` `info` gets company and product names from the old `forge.config.ts`;
  - point the darwin signing entitlements at `build/darwin/entitlements.plist`.

  Never copy the template's `main.go` or `frontend/`.
- [ ] **Step 2: Build flags.** Add the env-driven var `LDFLAGS: -X main.version={{.VERSION}} -X main.embeddedGitHubClientID={{.GITHUB_CLIENT_ID}} -X main.sentryDSN={{.SENTRY_DSN}}`, with `VERSION` defaulting to `dev` (from env `BISTRO_VERSION`), and append it to every `go build` in `build/*/Taskfile.yml`. The frontend build gets `BISTRO_VERSION` in its env.
- [ ] **Step 3: Root tasks.**
  - `generate` runs `wails3 generate bindings -clean=true -ts -i -d frontend/bindings`, then `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`.
  - `dev` runs the template's dev flow with `BISTRO_DATA_DIR={{.ROOT_DIR}}/.dev-data`.
  - Each `package:<os>` calls the template's per-OS package task, then produces exactly these names in `bin/`:

    | Task | Updater asset | Installer |
    |---|---|---|
    | darwin | `bistro-darwin-arm64.zip` (`ditto -c -k --keepParent bin/Bistro.app`; one top-level entry) | `bistro-darwin-arm64-installer.dmg` (template `package:dmg`, renamed) |
    | windows | `bistro-windows-amd64.zip` containing only `bistro.exe` | `bistro-windows-amd64-installer.exe` (NSIS, renamed) |
    | linux | `bistro-linux-amd64.tar.gz` containing only `bistro` | none |
- [ ] **Step 4: `main.go`.** Startup follows the spec's §1 order. Settings are loaded first because Sentry's consent reads them.
  ```go
  package main

  //go:embed all:frontend/dist
  var assets embed.FS

  // Set by -ldflags in release builds.
  var (
  	version                = "dev"
  	embeddedGitHubClientID = ""
  	sentryDSN              = ""
  )

  func main() {
  	if err := run(os.Args[1:]); err != nil {
  		fmt.Fprintln(os.Stderr, err)
  		os.Exit(1)
  	}
  }

  func run(args []string) error {
  	fs := flag.NewFlagSet("bistro", flag.ContinueOnError)
  	smoke := fs.Bool("smoke", false, "exit 0 once migrations ran and the window finished its first load")
  	if err := fs.Parse(args); err != nil {
  		return err
  	}
  	dataDir, err := resolveDataDir(*smoke)
  	if err != nil {
  		return err
  	}
  	log, closeLog, err := logging.Setup(os.Getenv("BISTRO_LOG_LEVEL"), filepath.Join(dataDir, "logs", "bistro.log"))
  	if err != nil {
  		return err
  	}
  	defer closeLog()
  	settings := shell.NewSettingsStore(filepath.Join(dataDir, "settings.json"), log)
  	if err := settings.Load(); err != nil {
  		return err
  	}
  	flushSentry, err := shell.InitSentry(sentryDSN, version, func() bool { return settings.Get().Privacy.TelemetryEnabled })
  	if err != nil {
  		log.Warn("sentry disabled", "err", err)
  	}
  	defer flushSentry()
  	reporter := shell.SentryReporter{Log: log}

  	// --smoke skips SingleInstance: with a real Bistro open, a smoke launch would
  	// otherwise focus it and exit 0 without exercising this build.
  	var single *application.SingleInstanceOptions
  	if !*smoke {
  		single = &application.SingleInstanceOptions{
  			UniqueID:               "app.bistro.review",
  			OnSecondInstanceLaunch: func(application.SecondInstanceData) { focusMainWindow() },
  		}
  	}
  	app := application.New(application.Options{
  		Name:   "Bistro",
  		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
  		SingleInstance: single,
  		PanicHandler: func(p *application.PanicDetails) { reporter.ReportPanic("wails", p.Error, []byte(p.FullStackTrace)) },
  		Mac:          application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
  	})

  	eng := engine.New(engine.Options{
  		DBPath:             filepath.Join(dataDir, "sync.db"),
  		Log:                log,
  		ReplayQueue:        true,
  		GitHubClientID:     cmp.Or(os.Getenv("GITHUB_CLIENT_ID"), embeddedGitHubClientID),
  		GitHubClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
  	})
  	eng.SetEmitter(services.EmitterFunc(func(name string, data any) { app.Event.Emit(name, data) }))
  	eng.SetPanicReporter(reporter)
  	ctx, cancel := context.WithCancel(context.Background())
  	defer cancel()
  	if err := eng.Start(ctx); err != nil {
  		return fmt.Errorf("start engine: %w", err)
  	}

  	set := eng.Services()
  	set.Shell = &services.Shell{Settings: settings, Platform: shell.NewPlatform(), EditorEnv: func() string { return os.Getenv("EDITOR") }, Version: version, LogDir: filepath.Join(dataDir, "logs"), Panics: reporter}
  	registerServices(app, set)

  	tel := shell.NewTelemetry(dataDir, log)
  	applySettings(eng, tel, log, settings.Get())
  	tel.TrackAppStarted(version)
  	settings.OnChange(func(prev, next shell.Settings) {
  		applySettings(eng, tel, log, next)
  		tel.TrackSettingsChanged(prev, next)
  		app.Event.Emit(services.EventSettingsChanged, next)
  	})
  	go settings.Watch(ctx, 2*time.Second)

  	setMenu(app)
  	window := newMainWindow(app, settings)
  	smokeOK := armSmoke(app, window, *smoke, log)
  	if !*smoke {
  		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
  			if err := shell.StartUpdater(ctx, app, version, log); err != nil {
  				log.Warn("updater disabled", "err", err)
  			}
  		})
  	}
  	app.OnShutdown(func() { cancel(); eng.Stop(); tel.Close() })
  	if err := app.Run(); err != nil {
  		return err
  	}
  	if *smoke && !smokeOK.Load() {
  		return errors.New("smoke: window never finished loading")
  	}
  	return nil
  }
  ```
  Supporting functions in `main.go`:
  - **`resolveDataDir(smoke bool)`**:
    - In smoke mode it always returns a fresh `os.MkdirTemp("", "bistro-smoke-*")` and ignores `BISTRO_DATA_DIR`. The smoke run skips `SingleInstance`, so it must never share a live profile's DB or keychain-backed sync. `run` removes that dir on exit.
    - Otherwise it returns `BISTRO_DATA_DIR` if set, else `filepath.Join(os.UserConfigDir(), "Bistro")`, and runs `MkdirAll` on it.
    - In smoke mode, also pass `ReplayQueue: false` and apply `AutoSync: false`, so a smoke run never talks to GitHub.
  - **`registerServices(app, set)`** makes one explicit `app.RegisterService(application.NewService(set.X))` call per service (21 calls). Write them out: the bindings generator finds services statically.
  - **`applySettings`** calls `eng.ApplySyncSettings(s.Sync.AutoSync, time.Duration(s.Sync.SyncInterval*float64(time.Second)))`, then `eng.ApplyClassificationSettings(services.ClassificationSettings{Mode: s.Classification.Mode, DismissalScope: s.Classification.DismissalScope, Provider: s.Classification.Provider, Model: s.Classification.Model, Endpoint: s.Classification.Endpoint})` (log on error), then `tel.SetEnabled(s.Privacy.TelemetryEnabled)`.
  - **`setMenu`** adds the `AppMenu` and `EditMenu` roles.
  - **`armSmoke`** returns an `*atomic.Bool`. When smoke is on, it handles `window.OnWindowEvent(events.Common.WindowRuntimeReady, …)` by storing true and calling `app.Quit()`, and sets `time.AfterFunc(60*time.Second, …)`, which logs and calls `os.Exit(2)`.

  If `logging.Setup` doesn't exist, move the old `setupLogging` from git history (`git show HEAD~N:cmd/bistro-sync/main.go`) into `internal/logging` with that signature.
- [ ] **Step 5: `window.go`.**
  - **`newMainWindow`** reads `settings.Get().Window`. With `Width == 0` it uses 1600×900, centred; otherwise `InitialPosition: application.WindowXY` with X, Y, Width and Height, and `StartState: WindowStateMaximised` when maximised. It sets `Name: "main"`, `Title: "Bistro"`, `URL: "/"`, `MinWidth: 960`, `MinHeight: 600`, and `Mac: application.MacWindow{TitleBar: application.MacTitleBarHiddenInset, InvisibleTitleBarHeight: 32}`.
  - **Persisting bounds.** On `events.Common.WindowDidMove` and `WindowDidResize`, a 500 ms `time.AfterFunc` debounce calls `settings.SetWindow(WindowBounds{…window.Bounds(), Maximised: window.IsMaximised()})`.
  - **`focusMainWindow()`** gets the window with `application.Get().Window.GetByName("main")`, then calls `Restore`, `Show` and `Focus`. Check the getter's return type in the module cache.
- [ ] **Step 6: Bindings.**
  - Run `wails3 task generate`. Check that `frontend/bindings/github.com/itterno/bistro/internal/services/` has one file per service (21) and that `eventdata.d.ts` lists every event in the [events table](inventory-engine.md#events).
  - **If a service or event is missing,** the generator did not see it. Fix the registration form (explicit `application.NewService(set.X)`), not the output.
  - Run `git add frontend/bindings` and commit it. If the pre-commit hook rejects a generated file as too large, add `frontend/bindings/` to the hook's generated-file skip list (next to the sqlc output rule) in this commit. Task 20 documents it.
- [ ] **Step 7: Frontend wiring.**
  - Add `"@wailsio/runtime": "3.0.0"`.
  - In `vite.config.ts`, add `wails('../bindings')` (from `@wailsio/runtime/plugins/vite`). The path is relative to `root: 'src'`; check the plugin's resolution and use the form that builds.
  - Add the alias `@bindings` → `bindings/github.com/itterno/bistro/internal` to vite, tsconfig (`"@bindings/*"`) and vitest.
- [ ] **Step 8: Verify.**
  ```bash
  wails3 task generate && git diff --exit-code frontend/bindings internal/store/db
  cd frontend && pnpm type-check && pnpm lint && pnpm test && pnpm build && cd ..
  go vet ./... && go test -race ./...
  wails3 task build && ./bin/bistro --smoke; echo "smoke exit=$?"   # macOS: bin/Bistro.app/Contents/MacOS/bistro if the build makes a bundle
  ```
  - Expected: no drift; every check passes; smoke prints `exit=0` within 60 s.
  - The renderer UI is not functional yet because it still speaks WS. That is expected until Task 16.
  - Also confirm the `go.mod` `ignore` directive did not break `//go:embed all:frontend/dist`: the build must embed the files.
- [ ] **Step 9: Commit.** `git add -A && git commit -m "feat: add Wails v3 entry point, Taskfile, packaging config and generated bindings"`

