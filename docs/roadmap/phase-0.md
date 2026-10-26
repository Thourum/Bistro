# Phase 0 — Foundation

[Roadmap](../ROADMAP.md)

## Phase 0 — Foundation (status: largely done)

This is the existing architecture. Most of it ships in pre-v1 builds. Listed here for completeness and to flag what still needs hardening before v1.0.

### Architecture in place

| Feature | Status | Notes |
|---|---|---|
| Two-process design (Go sync + Electron UI) | done | `go/cmd/bistro-sync` + `electron/src/main/go-spawn.ts`; stable enough to build on |
| SQLite-as-cache, Go-owns-writes | done | `go/internal/store`, migrations 000–011 |
| WebSocket transport over localhost | done | `go/internal/ws/server.go` enforces local origin; pin the port-binding strategy before v1 (see below) |
| Provider interface in `internal/provider` | done | Pre-built for multi-provider; GitHub is the only impl today (plus mock) |
| Migration runner with embedded SQL | done | Versioned, idempotent, tested |
| go-git for clone + unified diffs | done | go-git v5; foundation for every "since I last reviewed" feature later |
| OS keychain token storage | done | macOS / Windows / Linux all working via `go-keyring` (`go/internal/keychain`) |
| Structured slog logging with context propagation | done | `go/internal/logging/context.go`; `sync_id`, `repo`, `pr_id` etc carry through |
| Offline action queue | done | `store/action_queue.go` + migration 004; survives rate-limit and network failures |
| Crash monitoring (Go process → Electron) | done | `go-spawn.ts` onCrash + bounded restarts; UI shows error state instead of silent hang |
| Keyboard engine on TanStack Hotkeys | done (2026-06-12) | Custom matcher replaced; bindings are pure config in `utils/keybindings.ts`, dispatch/format via `@tanstack/react-hotkeys` |
| Sync-engine self-protection | done (2026-06-12) | Single-instance flock, busy_timeout, HTTP + per-PR timeouts, stdin parent-death watchdog, SYNC_ERROR broadcast |

### Foundation hardening before v1.0

| Item | Status | Action |
|---|---|---|
| WebSocket port collision | done | Go binds `:0` and prints `{"event":"ready","wsPort":N}`; Electron parses it and delivers the port to the renderer via preload additionalArguments |
| OAuth scope honesty | partial | Current spec says "minimal scopes, no write." That's incompatible with approve / comment / request-changes. Update to "scope per feature" model — see Phase 1 |
| `@pierre/diffs` license review | not started | Verify the license terms before v1.0 ships publicly (currently `@pierre/diffs@^1.1.15`) |
| Initial-launch SQLite migration race | done | The ready signal is only emitted after migrations complete and the WS listener is up; Electron blocks on it |
| `pr_diffs` table size budget | done (2026-07-11) | BIS-25: diff budgets enforced, partial manifest persisted with per-file metadata + diff_status (migrations 024/025, `git/diff.go`, `sync/fetcher.go`) |
| `repos` watched-flag UI exposure | partial | Already in schema; surface a per-repo "last sync time / error / rate-limit remaining" indicator |

---

