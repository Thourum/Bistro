# Bistro agent guide

## What & why

**Bistro** is the keyboard-first code review client for the AI-PR era: a local-first desktop GitHub PR review app. Review PRs at the speed of thought — no browser, no mouse.

There is no server. Everything runs locally. SQLite is a local cache, not a shared database.

Authoritative references (read when relevant, don't duplicate here):

- Product overview: `README.md`
- Go backend package-by-package map: `docs/go-project-overview.md`
- Roadmap / v1.0 status: `docs/ROADMAP.md`
- Design system & UI specs: `docs/DESIGN.md` (UI mockups live in `electron/stitch_terminal_authentic_prd/`)

This project is early and evolving. If you change structure, commands, or conventions, update this file in the same change.

## Architecture

Two-process desktop app:

1. **Go sync engine** (`bistro-sync`) — authenticates to GitHub via OAuth; syncs PR metadata, comments, CI status, and diffs into local SQLite; generates diffs locally via `go-git`; exposes a localhost WebSocket API; queues write actions (approve / request-changes / comment) offline with retry. **Owns all SQLite writes.**
2. **Electron + React UI** — requests data over WebSocket; receives push events; keyboard-first navigation via TanStack Hotkeys.

Process lifecycle: Electron Forge spawns `bistro-sync`, or connects to an already-running engine on the WS port (default `8080`). The engine prints `{"event":"ready","wsPort":N}` to stdout after migrations, holds a flock on `<db>.lock` (single-instance), and watches stdin (`-exit-on-stdin-close`) so it dies when Electron dies. Deeper detail: `docs/go-project-overview.md`.

Where things live: Go engine in `go/` (entry `go/cmd/bistro-sync/`, packages under `go/internal/`); Electron UI in `electron/src/` (`main/` = process + spawn + IPC + settings, `renderer/` = React app).

## Tech stack

- **Go 1.26** — go-git, go-github, gorilla/websocket, mattn/go-sqlite3, go-keyring, x/oauth2.
- **Electron + Electron Forge + Vite** — React 19, TypeScript, TanStack (React Query / Hotkeys / Virtual / Store), Radix UI, Tailwind, Biome (lint+format), Vitest (test).
- **Package manager: pnpm** (pnpm workspace). Versions live in `go/go.mod` and `electron/package.json` — check there, don't assume.

## Conventions

- **Error handling: in repository TS/JS and Go code, use `neverthrow`. NEVER write `try`/`catch` for control flow or error handling.** Return and chain `Result` / `ResultAsync` instead. (The only `try/catch` allowed anywhere is inside throwaway context-mode MCP sandbox scripts — never in committed code.)
- Go owns all SQLite writes — the UI never writes the DB directly.
- SQLite is a cache; the WebSocket is localhost-only (origin-enforced); migrations are versioned, idempotent, embedded.
- Keyboard bindings are pure config in `electron/src/renderer/utils/keybindings.ts`; registration in `hooks/useAppHotkeys.ts`.
- Do not assert a Go module path from memory — read it from `go/go.mod`.

## How to work

There is no Makefile. The commands below are authoritative; ignore `make` targets in any older docs.

Normal dev — launch the Electron app; Forge handles the Go engine (spawns one, or connects to an engine you already have running):

```bash
cd electron && pnpm start
```

Run the Go sync engine standalone (e.g. to watch its logs) — Electron will connect to it automatically:

```bash
cd go && go build -o bistro-sync ./cmd/bistro-sync && ./bistro-sync
# or without building a binary:
go run cmd/bistro-sync/main.go
```

Config: copy `electron/.env.example` to `electron/.env.local` (GitHub OAuth client id/secret, WS port, log level, window size).

Verify a change before declaring done:

```bash
# Go
cd go && go vet ./... && go test -race ./...
# Electron
cd electron && pnpm type-check && pnpm lint && pnpm test
```

If a command or detail is unclear, check `electron/package.json` scripts. Do not invent workflows.

---

# Agent harness rules: context-mode MCP

context-mode MCP tools available. Rules protect context window from flooding. One unrouted command dumps 56 KB into context.

## Think in Code — MANDATORY

Analyze/count/filter/compare/search/parse/transform data: write code via context-mode_ctx_execute(language, code), console.log() only the answer. Do NOT read raw data into context. PROGRAM the analysis, not COMPUTE it. Pure JavaScript — Node.js built-ins only (fs, path, child_process). Handle null/undefined. One script replaces ten tool calls. (Throwaway sandbox scripts may use try/catch; committed code may not — see Conventions.)

## BLOCKED — do NOT attempt

### curl / wget — BLOCKED. Use context-mode_ctx_fetch_and_index(url, source) or context-mode_ctx_execute.
### Inline HTTP — BLOCKED. Do not use fetch('http, requests.get(, requests.post(, http.get(, http.request(. Use context-mode_ctx_execute.
### Direct web fetching — BLOCKED. Use context-mode_ctx_fetch_and_index then context-mode_ctx_search.

## REDIRECTED — use sandbox

### Shell (>20 lines output): Shell ONLY for git, mkdir, rm, mv, cd, ls, npm install, pip install. Otherwise use context-mode_ctx_batch_execute or context-mode_ctx_execute(language: "shell").
### File reading for analysis: Reading to edit → reading correct. Reading to analyze/explore/summarize → context-mode_ctx_execute_file(path, language, code).
### grep / search with large results: context-mode_ctx_execute(language: "shell", code: "grep ...").

## Tool selection

0. MEMORY: context-mode_ctx_search(sort: "timeline") — after resume, check prior context before asking user.
1. GATHER: context-mode_ctx_batch_execute(commands, queries) — runs all commands, auto-indexes, returns search. Each command {label, command}.
2. FOLLOW-UP: context-mode_ctx_search(queries: [...]) — all questions as array, ONE call.
3. PROCESSING: context-mode_ctx_execute / context-mode_ctx_execute_file — sandbox, only stdout enters context.
4. WEB: context-mode_ctx_fetch_and_index(url, source) then context-mode_ctx_search(queries).
5. INDEX: context-mode_ctx_index(content, source) — store in FTS5 for later search.

## Parallel I/O batches

For multi-URL fetches or multi-API calls, always include concurrency: N (1-8). Use 4-8 for I/O-bound, keep 1 for CPU-bound or shared-state. GitHub API rate-limit: cap at 4 for gh calls.

## Output

Write artifacts to FILES — never inline. Return: file path + 1-line description. Descriptive source labels for search(source: "label").

## Session Continuity

Skills, roles, and decisions persist for the entire session. Do not abandon them as the conversation grows.

## Memory

Session history is persistent and searchable. On resume, search BEFORE asking the user: decisions via source: "decision", constraints via source: "constraint". DO NOT ask "what were we working on?" — SEARCH FIRST. If search returns 0 results, proceed as a fresh session.
