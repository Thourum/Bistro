# Bistro

Keyboard-first desktop code review tool for GitHub (GitLab and Gitea planned).

## Architecture

```
GitHub API
    |
    v
Go sync engine (bistro-sync)
    |-- fetches PRs, diffs via go-git
    |-- writes to SQLite (local cache)
    |-- emits real-time events over WebSocket
    |
    v
Electron + React UI
    |-- reads SQLite directly for instant rendering
    |-- receives WebSocket events for live updates
    |-- vim-inspired keyboard navigation
```

No server. Everything runs locally. SQLite is a cache, not a shared database.

## Prerequisites

- Go 1.21+
- Node 18+
- pnpm

## Quick Start

```bash
# Install all dependencies
make install

# Run tests
make test

# Start dev mode (two terminals)
make build-go                         # Terminal 1
cd go && ./bistro-sync --log-level debug

cd electron && pnpm dev               # Terminal 2
```

## Project Structure

```
bistro/
├── go/                        Go sync engine
│   ├── cmd/bistro-sync/       Binary entry point
│   └── internal/
│       ├── provider/github/   GitHub API client
│       ├── sync/              PR sync orchestration
│       ├── git/               Diff generation (go-git)
│       ├── store/             SQLite read/write
│       ├── ws/                WebSocket server
│       ├── actions/           Approve, comment, request-changes
│       ├── events/            Event types
│       ├── keychain/          OS keychain token storage
│       └── server/            App orchestrator
├── electron/                  Electron + React UI
│   ├── src/renderer/          React app (pages, components, queries)
│   ├── components/ui/         Shared UI components
│   └── tests/                 Unit and integration tests
└── design/                    Architecture and API docs
    ├── api-contracts.md        WebSocket message schemas
    ├── data-flow.md            End-to-end data lifecycle
    ├── keyboard-bindings.md    Key mapping reference
    ├── error-handling.md       Error types and messages
    └── security-model.md       Token storage and auth
```

## Configuration

### Electron

Copy `electron/.env.example` to `electron/.env.local` and fill in values:

```
WS_PORT=8080                   # Must match --ws-port on the Go side
GO_LOG_LEVEL=info              # debug | info | warn | error
GITHUB_CLIENT_ID=              # From https://github.com/settings/developers
GITHUB_CLIENT_SECRET=          # Keep secret, never commit
WINDOW_WIDTH=1600
WINDOW_HEIGHT=900
```

### Go sync engine flags

```
--ws-port       int     WebSocket port (default: 8080)
--db-path       string  SQLite database path (default: ./data/sync.db)
--log-level     string  debug | info | warn | error (default: info)
--log-file      string  Log file path; omit to use stderr only
--replay-queue  bool    Replay offline action queue on startup (default: true)
```

Example:

```bash
./bistro-sync --log-level debug --log-file logs/sync.log
```

## Testing

```bash
make test              # All tests
make test-go           # Go tests only
make test-electron     # Electron tests only

# Go with race detector
cd go && go test -race ./...
```

## Key Design Decisions

- **SQLite as local cache** — the UI reads directly from SQLite for zero-latency rendering; the Go engine owns all writes.
- **Vim-inspired keyboard bindings** — no mouse required; `j/k` to navigate, `:` for command palette, `?` for help.
- **Go for parallel API fanout** — concurrent goroutines fetch PR metadata, diffs, and comments; Electron stays thin.
- **neverthrow-style Result types** — explicit error handling throughout the Electron codebase; no unhandled promise rejections.
- **Offline queue** — actions (approve, comment) are queued in SQLite when the API is unavailable and replayed on reconnect.
