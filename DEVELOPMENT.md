# Developing Bistro

Technical guide for building, running, and contributing to Bistro. For the
product overview, see the [README](README.md); for the feature plan, see
[docs/ROADMAP.md](docs/ROADMAP.md).

## Architecture

```
GitHub API
    |
    v
Go sync engine (bistro-sync)
    |-- fetches PRs, reviews, comments via the GitHub API
    |-- clones PR base/head and generates diffs locally via go-git
    |-- writes to SQLite (local cache, engine owns all writes)
    |-- emits real-time events over a localhost WebSocket
    |
    v
Electron + React UI
    |-- requests data over WebSocket (REQUEST_PR_LIST, REQUEST_DIFF, …)
    |-- receives push events for live updates (SYNC_COMPLETE, SYNC_ERROR, …)
    |-- keyboard-first navigation via TanStack Hotkeys
```

No server. Everything runs locally. SQLite is a cache, not a shared database.

### Process lifecycle

- Electron spawns `bistro-sync` with `-ws-port 0`; the engine binds a random
  port and prints `{"event":"ready","wsPort":N}` to stdout **after** SQLite
  migrations complete. Electron blocks on this line, then passes the port to
  the renderer synchronously via preload `additionalArguments`.
- The engine holds a `flock` on `<db>.lock` — a second instance on the same
  database exits immediately with a clear error.
- The engine watches stdin (`-exit-on-stdin-close`): if the Electron process
  dies — however it dies — the pipe closes and the engine exits, so orphaned
  processes can't hold the database.

## Prerequisites

- Go 1.26.x, matching `go/go.mod`
- Node 18+
- pnpm (the workspace uses `nodeLinker: hoisted` — required by electron-forge)

## Quick Start

```bash
# Install Electron dependencies
cd electron && pnpm install

# Build the Go sync engine
pnpm build

# Run the app (spawns the Go engine automatically)
pnpm start
```

To run the Go engine separately (e.g. under Delve), set
`BISTRO_SKIP_GO_SPAWN=1` for Electron and start the engine yourself:

```bash
cd go && ./bistro-sync -ws-port 8080 -log-level debug
BISTRO_SKIP_GO_SPAWN=1 WS_PORT=8080 pnpm start   # in electron/
```

## Project Structure

```
bistro/
├── go/                        Go sync engine
│   ├── cmd/bistro-sync/       Binary entry point
│   └── internal/
│       ├── provider/          Provider interface + github/ implementation
│       ├── sync/              PR sync orchestration, worker pool, writer
│       ├── git/               Clone + diff generation (go-git)
│       ├── store/             SQLite read/write, embedded migrations
│       ├── ws/                WebSocket server + dispatcher
│       ├── actions/           Approve, comment, request-changes (+ offline queue)
│       ├── logging/           slog context propagation helpers
│       ├── keychain/          OS keychain token storage
│       └── server/            App orchestrator, sync loop, WS handlers
├── electron/                  Electron + React UI
│   ├── src/main/              Main process (go-spawn, IPC, settings)
│   ├── src/renderer/          React app (pages, components, hooks, stores)
│   │   ├── utils/keybindings.ts   All keyboard bindings (pure config)
│   │   └── hooks/useAppHotkeys.ts Registration via TanStack Hotkeys
│   └── tests/unit/            Vitest suites
├── design/                    Architecture and API docs
└── docs/                      ROADMAP.md, session UPDATE files
```

## Configuration

### Electron environment (`electron/.env.local`)

```
WS_PORT=0                      # 0 = engine picks a free port (default)
GO_LOG_LEVEL=debug             # dev default is debug; prod default info
GITHUB_CLIENT_ID=              # From https://github.com/settings/developers
GITHUB_CLIENT_SECRET=          # Keep secret, never commit
WINDOW_WIDTH=1600
WINDOW_HEIGHT=900
```

### Go sync engine flags

```
-ws-port              int     WebSocket port; 0 = random, reported via ready signal (default: 8080)
-db-path              string  SQLite database path (default: ./data/sync.db)
-log-level            string  debug | info | warn | error (default: info)
-log-file             string  Log file path; omit to use stderr only
-replay-queue         bool    Replay offline action queue on startup (default: true)
-exit-on-stdin-close  bool    Exit when stdin closes (set by Electron)
```

### User settings

The app writes `settings.json` to Electron's `userData` directory (open it via
`:settings` → "Open settings file"). Keybinding overrides map a stable
`actionId` to an array of TanStack hotkey strings:

```json
{
  "keybindings": {
    "list.down": ["x"],          // rebind to a single key
    "list.top": ["g", "g"],      // two entries = a sequence
    "list.star": []              // empty = unbind
  }
}
```

Action ids and defaults live in `electron/src/renderer/utils/keybindings.ts`.

## Testing

```bash
(cd go && go test ./...) && (cd electron && pnpm test)    # All tests
cd go && go test ./...                                    # Go tests only
cd electron && pnpm test                                  # Electron tests only

cd go && go test -race ./...                          # race detector
cd electron && pnpm exec tsc --noEmit && pnpm test    # typecheck + vitest
```

## Packaging

```bash
cd electron && pnpm package     # package the Electron app locally
cd electron && pnpm release:macos   # create a macOS DMG
cd electron && pnpm release:win     # create a Windows zip
```

## Debugging

- Dev builds spawn the engine at `-log-level debug`: every sync phase, per-PR
  fetch step, and GitHub HTTP call (with rate-limit headers) is logged.
- Engine logs go to stderr (visible in the Electron terminal) and to
  `go/logs/bistro-sync.log` in dev.
- Renderer request timeouts report the socket state and pending request count.
- Sync failures are broadcast to the UI as `SYNC_ERROR` — a silent hang is a
  bug, not a known failure mode.
- "another bistro-sync instance is already using …" on startup means a
  previous engine still holds the DB lock — find it with
  `lsof go/sync.db` and kill it.

## Key Design Decisions

- **SQLite as local cache** — the Go engine owns all writes; the UI reads via
  WebSocket request/response against the same cache. WAL mode, pooled
  connections, bounded `busy_timeout`.
- **Local diff generation** — PR base/head are cloned (cached) and diffed with
  go-git, with clone auth from the OS keychain. This is the foundation for
  rebase-surviving review features (see ROADMAP Phase 2).
- **Keyboard config as data** — bindings are pure config (`keybindings.ts`);
  dispatch, sequence handling, and display formatting are delegated to
  TanStack Hotkeys. The footer legend, help overlay, and settings page all
  derive from the same config, so remaps show up everywhere.
- **Offline queue** — actions (approve, comment) are queued in SQLite when the
  API is unavailable and replayed on startup/reconnect.
- **Virtualized diff canvas** — TanStack Virtual mounts only the file blocks
  near the viewport, so 100+ file diffs stay responsive.
