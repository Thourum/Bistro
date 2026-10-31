# Developing Bistro

For product context, see [README.md](README.md). [CLAUDE.md](CLAUDE.md) is the authoritative agent guide for conventions and verification; this document covers local setup and operation.

## Architecture

Bistro is one Wails v3.0.0 application process. `main.go` embeds the Vite build, creates the native window, starts `internal/engine`, and binds `internal/services`. The React renderer calls generated TypeScript bindings through `frontend/src/services/engine/call.ts` and receives typed Wails events. Go owns all SQLite writes, local Git operations, sync, and offline action replay. `internal/shell` owns settings, platform/editor integration, telemetry, and updates.

Wails handles single-instance enforcement. `OnShutdown` cancels the shared engine/shell context and stops updater checks; deferred `engine.Stop()` waits for background work and closes SQLite after the Wails run loop returns. There is no standalone sync binary, ready-line handshake, engine port, or WebSocket transport. Only sign-in uses a temporary OAuth loopback callback listener.

See [the Go package map](docs/go-project-overview.md) for backend details and [ADR 0001](docs/adr/0001-electron-vs-wails.md) for the historical decision and migration amendment.

## Background mode

When the tray is available on macOS or Windows, closing the window keeps Bistro running and syncing in the tray. On macOS, this also hides the Dock icon. Use **Show Bistro** in the tray to restore the window, or launch Bistro again. To exit, use ⌘Q on macOS, the app menu's **Quit**, or the tray's **Quit Bistro**.

On Linux, closing the window quits even if the tray icon appears, because Wails does not report tray readiness there. Under `--smoke`, Bistro creates no tray and closing the window quits.

## Prerequisites

- Go 1.26.1, matching root `go.mod`, with CGO and a native C compiler.
- Node compatible with installed Vite (`^20.19.0 || >=22.12.0`) and pnpm. The root lockfile and workspace cover `frontend/` and `website/`.
- Wails CLI pinned to the Go runtime version:

  ```bash
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
  ```

  Ensure Go's binary directory is on PATH. The CLI includes its task runner; no separate `task` binary is needed.
- macOS: Xcode Command Line Tools; the native webview is WKWebView.
- Windows: a CGO-capable compiler and WebView2; packaging also needs NSIS. The installer includes the WebView2 bootstrapper.
- Linux: GTK 4 / WebKitGTK 6.0 development libraries and keyring support. The release workflow installs `build-essential`, `ca-certificates`, `dbus-x11`, `gnome-keyring`, `libsecret-1-dev`, `libgtk-4-dev`, and `libwebkitgtk-6.0-dev` on Ubuntu.

## Quick start

From the repository root:

```bash
pnpm install
wails3 task dev
```

Development runs the native app with Vite and stores local state in `.dev-data/`. Sign-in works out of the box through Bistro's own OAuth app, with code exchange via `https://get-bistro.com/api/oauth/token`. `GITHUB_CLIENT_ID` overrides the embedded client ID. Only when both `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET` are set in the process environment does code exchange go directly to GitHub for your custom OAuth app. Do not commit secrets.

## Project structure

```text
main.go, window.go     Wails entrypoint, lifecycle, native window/menu
internal/engine/       In-process engine and domain wiring
internal/services/     Typed methods, errors, registered events
internal/shell/        Settings, editor/platform, telemetry, updater
internal/prsync/       Sync orchestration, workerpool, writer
internal/git/          Local clone, diff, anchor, and cache operations
internal/store/        SQLite, migrations, SQL queries, generated sqlc output
frontend/src/          React pages, components, hooks, stores, services, utils
frontend/bindings/     Committed generated Wails TypeScript bindings
frontend/tests/        Vitest support and behavioral suites
build/                 Platform Taskfiles, icons, signing/installer resources
scripts/smoke.sh        Bounded native smoke check
website/               Documentation/marketing site (separate workspace package)
docs/                  Design, roadmap, architecture decisions
```

Frontend layout guidance lives in [frontend/CLAUDE.md](frontend/CLAUDE.md); detailed conventions live in the root guide.

## Configuration

| Variable | Meaning |
|---|---|
| `BISTRO_DATA_DIR` | Override the app data directory; dev sets it to repo-root `.dev-data/`. |
| `GITHUB_CLIENT_ID` | Optional override of Bistro's embedded public OAuth client ID, at build time or runtime. |
| `GITHUB_CLIENT_SECRET` | OAuth secret for direct GitHub exchange only when `GITHUB_CLIENT_ID` is also set in the environment; never embed or commit it. |
| `BISTRO_LOG_LEVEL` | `debug`, `info`, `warn`, or `error`; default `info`. |
| `BISTRO_VERSION` | Build-time version stamp; default `dev`, which disables updates. |

Without an override, data lives under `os.UserConfigDir()/Bistro`: `~/Library/Application Support/Bistro/` on macOS, `%AppData%\Bistro\` on Windows, and `$XDG_CONFIG_HOME/Bistro/` (or `~/.config/Bistro/`) on Linux. It contains `sync.db`, `settings.json`, and `logs/bistro.log`. GitHub tokens stay in the OS keychain.

Settings changes are validated and atomically persisted by Go; external edits are polled. Open the file from Settings. Keyboard overrides map action IDs to hotkey arrays: one entry is a key, multiple entries form a sequence, and an empty array unbinds. Action IDs live in `frontend/src/utils/keybindings.ts`, with defaults per navigation style in `utils/keymaps.ts`.

## Generation and verification

```bash
# Root: regenerate committed Wails bindings and sqlc output
wails3 task generate

# Root: backend checks
go vet ./... && go test -race ./...

# Renderer checks
cd frontend && pnpm type-check && pnpm lint && pnpm test
```

For SQL-only generation, edit `internal/store/queries/*.sql` and run `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` from the root. Never hand-edit `internal/store/db/` or `frontend/bindings/`.

The workspace's seven-day `minimumReleaseAge` policy exempts exactly `'@wailsio/runtime@3.0.0'`, approved on 2026-10-29 to keep the JS runtime aligned with Go. See [CLAUDE.md](CLAUDE.md#tech-stack) before changing dependency policy.

## Packaging

```bash
# Root: build for the host
wails3 task build

# All three platforms, in order: Darwin, Windows, then Linux
wails3 task package

# One platform
wails3 task package:darwin
wails3 task package:windows
wails3 task package:linux

# Prepare the Docker image used by cross-build tasks
wails3 task setup:docker
```

`Taskfile.yml` and platform Taskfiles in `build/` are authoritative. `package` runs the Darwin, Windows, and Linux tasks in that order. macOS packaging is native and arm64; Windows and Linux release artifacts target amd64. Docker-backed cross-builds require Docker and the prepared image; macOS bundles still require macOS tooling. A built frontend is embedded in the executable.

For a versioned local macOS bundle and smoke check:

```bash
BISTRO_VERSION=0.0.0-local wails3 task package:darwin
scripts/smoke.sh bin/Bistro.app
```

The smoke script also accepts `bin/bistro.exe` or `bin/bistro`. It runs `--smoke` with a bounded timeout, isolated temporary data, and no sync/replay/telemetry/updates. Linux needs `xvfb-run`; macOS needs GNU `timeout` or `gtimeout`. This is a native-window/runtime check, not an authenticated review-flow test.

The [release workflow](.github/workflows/release.yml) runs security gates, builds and smoke-tests all three platforms, and creates a draft pre-release with platform archives, installers, and `SHA256SUMS`. Signing/notarization is conditional on configured secrets. Installation and updater behavior are in [docs/ALPHA.md](docs/ALPHA.md). Publishing or pushing a release tag requires explicit permission.

## Debugging

- Set `BISTRO_LOG_LEVEL=debug` for scoped engine logs. Logs go to stderr and `<data-dir>/logs/bistro.log`; review them for private data before sharing.
- Renderer calls use structured service codes/reasons, cancellation, and a default 30-second timeout. Auth and folder scans deliberately have no renderer timeout.
- Typed sync errors reach the UI through registered events. A silent hang is a bug, not expected behavior.
- A second normal launch focuses the running window. Use isolated `--smoke` for automated native checks rather than competing for app state.

## Key design decisions

- **SQLite as local cache:** Go owns writes, WAL mode, pooled connections, and bounded busy timeouts; the UI reads via typed services.
- **Local diffs:** go-git clones/caches PR commits and generates merge-base diffs, with patch fingerprints for rebase-stable viewed marks.
- **Keyboard config as data:** palette, help, footer, and settings derive from the same action catalog and effective keymaps.
- **Offline replay:** replayable actions queue in SQLite when GitHub is unavailable. Draft review submission does not queue and preserves drafts on failure.
- **Virtualized diffs:** only nearby file blocks mount, keeping large reviews responsive.
