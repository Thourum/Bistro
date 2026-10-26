# Bistro agent guide

## What & why

**Bistro** is the keyboard-first code review client for the AI-PR era: a local-first desktop GitHub PR review app. Review PRs at the speed of thought — no browser, no mouse.

There is no server. Everything runs locally. SQLite is a local cache, not a shared database. The one remote call besides GitHub is the OAuth code exchange: when no `GITHUB_CLIENT_SECRET` is configured (packaged builds), the engine posts the code to `https://get-bistro.com/api/oauth/token`, which holds the secret.

Authoritative references (read when relevant, don't duplicate here):

- Product overview: `README.md`
- Go backend package-by-package map: `docs/go-project-overview.md`
- Roadmap / v1.0 status: `docs/ROADMAP.md`
- Design system & UI specs: `docs/DESIGN.md` (UI mockups live in `electron/stitch_terminal_authentic_prd/`)
- User facing documentation: `website/src/pages/docs/`

This project is early and evolving. If you change structure, commands, or conventions, update this file in the same change.

## Architecture

Two-process desktop app:

1. **Go sync engine** (`bistro-sync`) — authenticates to GitHub via OAuth; syncs PR metadata, comments, CI status, and diffs into local SQLite; generates diffs locally via `go-git`; exposes a localhost WebSocket API; queues write actions (approve / request-changes / comment) offline with retry. **Owns all SQLite writes.**
2. **Electron + React UI** — requests data over WebSocket; receives push events; keyboard-first navigation via TanStack Hotkeys.

Process lifecycle: Electron Forge spawns `bistro-sync`, or connects to an already-running engine on the WS port (default `8080`). The engine prints `{"event":"ready","wsPort":N}` to stdout after migrations, holds a flock on `<db>.lock` (single-instance), and watches stdin (`-exit-on-stdin-close`) so it dies when Electron dies. Deeper detail: `docs/go-project-overview.md`.

Where things live: Go engine in `go/` (entry `go/cmd/bistro-sync/`, packages under `go/internal/`); Electron UI in `electron/src/` (`main/` = process + spawn + IPC + settings, `renderer/` = React app).

Within `go/internal/server/`, `app.go` owns the App struct, constructors, store preparation, and Run lifecycle; provider lifecycle, configuration, LLM setup, expertise, event bridging, repo-location wiring, and handler registration live in `provider.go`, `config.go`, `llm.go`, `expertise.go`, `bridge.go`, `repo_location.go`, and `handlers.go` respectively. All remain in package `server`.

Local diff generation, interdiff, and patch fingerprints live in `internal/git/diff` (`Generate`, `Interdiff`, `PatchID`). Interdiff takes the shared `*repolock.Locks` used by clone/fetch; it does not depend on `git.Client`. See the [Go package layout spec](docs/superpowers/specs/2026-10-26-go-package-layout-design.md) for the package boundaries.

Clone cache entries, path/staging helpers, maintenance, and PR-head retention live in `internal/git/clonecache`; it depends on `remote`, never the parent `git` package. Shared on-disk commit fixtures live in the test-support package `internal/git/gittest`, which depends only on go-git and the standard library. See [Go package layout spec](docs/superpowers/specs/2026-10-26-go-package-layout-design.md) for the cache API and boundaries.

Go tests outside package `store` share migrated in-memory databases via `internal/store/storetest.Open(t)`; `store`'s own internal tests keep local helpers to avoid an import cycle. See the [Go package layout spec](docs/superpowers/specs/2026-10-26-go-package-layout-design.md) for the shared-leaf refactor.

## Tech stack

- **Go 1.26** — go-git, go-github, gorilla/websocket, mattn/go-sqlite3, go-keyring, x/oauth2.
- **Electron + Electron Forge + Vite** — React 19, TypeScript, TanStack (React Query / Hotkeys / Virtual / Store), Radix UI, Tailwind, Biome (lint+format), Vitest (test).
- **Package manager: pnpm** for the Electron app. Run pnpm commands from `electron/`; the lockfile is the repo-root `pnpm-lock.yaml` (workspace defined in `pnpm-workspace.yaml`, covering `electron/` and `website/`). `minimumReleaseAge` in `pnpm-workspace.yaml` blocks package versions younger than 7 days, so a just-published release won't resolve yet. Versions live in `go/go.mod` and `electron/package.json` — check there, don't assume.

## Conventions

- **Error handling: in Electron TS/JS code, use `@thourum/fault`. NEVER write `try`/`catch` for control flow or error handling.** Return and chain `Result<T, Fault>` / `ResultAsync<T, Fault>` with tagged faults; narrow errors with `fault.tag` and use `fault.message` at display/log boundaries. Go continues to use its existing error handling. (The only `try/catch` allowed anywhere is inside throwaway context-mode MCP sandbox scripts — never in committed code.)
- Go owns all SQLite writes — the UI never writes the DB directly.
- SQLite is a cache; the WebSocket is localhost-only (origin-enforced); migrations are versioned, idempotent, embedded.
- Keyboard bindings are pure config, no handlers: actions (`actionId`, label, activation) in `ACTION_CATALOG` in `electron/src/renderer/utils/keybindings.ts`, keys per navigation style (`vim` / `emacs` / `standard`) in `utils/keymaps.ts` (`KEYMAPS`, `SEQUENCE_TIMEOUT_MS`). `getEffectiveBindings(style, overrides)` resolves user override → style table → unbound; handlers live in `utils/keybindingActions.ts`, enable/scope rules in `utils/keybindingActivation.ts`; registration in `hooks/useAppHotkeys.ts`. Don't register hotkeys outside `useAppHotkeys` (textarea-local submit uses `TEXTAREA_SUBMIT_HOTKEY` / `isTextareaSubmit` from the same file).
- Validate external data with zod at the edge, never `as` casts: WS envelopes and push events via `dataSchemas`/`parseWsMessage` in `types/wsSchemas.ts`; `wsClient.request()` responses via schemas in `types/index.ts` + `parseResponse` (`utils/error.ts`).
- WebSocket transport lives in `services/ws/connection.ts`: use `wsClient.send` / `wsClient.request` directly, with tagged `ResultAsync` request errors. Only App's Bootstrap calls `hooks/useWebSocketConnection.ts` to own connect/disconnect. Push-event side effects live in `services/ws/events/<domain>.ts`, with an exhaustive handler table in `services/ws/events/index.ts`.
- Renderer hooks own React state/effects and data-fetching lifecycles; plain WS/IPC requests and appStore writes live in `services/` (`readState.ts`, `prActions.ts`, `dismissals.ts`, `editor.ts`). Call services directly; don't wrap them in hooks solely for `useCallback`.
- Large renderer pages keep feature-local pieces beside them: `pages/settings/*Section.tsx` for Settings, `use<Page>Data.ts` / `use<Page>Actions.ts` hooks and `*Sections.tsx` / `*Chrome.tsx` for PR detail and diff pages.
- Onboarding and the first-run rail (design: `docs/superpowers/specs/2026-10-24-first-run-onboarding-rail-design.md`): pure models in `utils/onboardingRepos.ts`, `utils/onboardingSandbox.ts`, `utils/firstRunRail.ts`; WS/appStore writes in `services/onboardingSync.ts` and `services/firstRunRail.ts`. Engine side (`go/internal/server/handler/`): `review_queue.go` (`REQUEST_REVIEW_QUEUE`; repo IDs are lowercase `identity.ID()`, e.g. `github:owner/name`), `dev_folder_scan.go` (`SCAN_DEV_FOLDER` / `SCAN_CANCEL`; Stop cancels only its own scan, whose request id comes from `wsClient.request(..., { onSent })`), and `REQUEST_REPO_LIST {cache_only}` for a provider-free cached read. Components that show or hide UI by a binding's enable rule use `useBindingEnabled` from `hooks/useAppHotkeys.ts`.
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

Adding or changing a SQL query: edit/add a `.sql` file under `go/internal/store/queries/`, then regenerate (`sqlc.yaml` is at `go/`):

```bash
cd go && go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
```

Generated code lives in `go/internal/store/db/` — never hand-edit it. If a migration lands without a matching `sqlc generate` run, the generated structs silently drift from the schema (this happened once — migrations 033/034 added columns/tables that `internal/store/db/models.go` didn't know about until the next `sqlc generate`).

Verify a change before declaring done:

```bash
# Go
cd go && go vet ./... && go test -race ./...
# Electron
cd electron && pnpm type-check && pnpm lint && pnpm test
```

Enable the pre-commit hook once per clone (blocks staged files over 5,000 tokens, bytes/4; skips lockfiles, sqlc output, images, files marked generated; the error message explains how to split):

```bash
git config core.hooksPath .githooks
```

If a command or detail is unclear, check `electron/package.json` scripts. Do not invent workflows.

---

# context-mode — MANDATORY routing rules

context-mode MCP tools available. Rules protect context window from flooding. One unrouted command dumps 56 KB into context.

## Think in Code — MANDATORY

Analyze/count/filter/compare/search/parse/transform data: **write code** via `ctx_execute(language, code)`, `console.log()` only the answer. Do NOT read raw data into context. PROGRAM the analysis, not COMPUTE it. Pure JavaScript — Node.js built-ins only (`fs`, `path`, `child_process`). `try/catch`, handle `null`/`undefined`. One script replaces ten tool calls.

## BLOCKED — do NOT attempt

### curl / wget — BLOCKED
Intercepted and replaced with error. Do NOT retry.
Use: `ctx_fetch_and_index(url, source)` or `ctx_execute(language: "javascript", code: "const r = await fetch(...)")`

### Inline HTTP — BLOCKED
`fetch('http`, `requests.get(`, `requests.post(`, `http.get(`, `http.request(` — intercepted. Do NOT retry.
Use: `ctx_execute(language, code)` — only stdout enters context

### WebFetch — BLOCKED
Use: `ctx_fetch_and_index(url, source)` then `ctx_search(queries)`

## REDIRECTED — use sandbox

### Bash (>20 lines output)
Bash ONLY for: `git`, `mkdir`, `rm`, `mv`, `cd`, `ls`, `npm install`, `pip install`.
Otherwise: `ctx_batch_execute(commands, queries)` or `ctx_execute(language: "javascript", code: "...")`. Use `language: "shell"` only when code matches the host shell.

### Read (for analysis)
Reading to **Edit** → Read correct. Reading to **analyze/explore/summarize** → `ctx_execute_file(path, language, code)`.

### Grep — may flood context
Use `ctx_execute(language: "javascript", code: "...")` in sandbox for portable filtering/counting.

## Tool selection

0. **MEMORY**: `ctx_search(sort: "timeline")` — after resume, check prior context before asking user.
1. **GATHER**: `ctx_batch_execute(commands, queries)` — runs all commands, auto-indexes, returns search. ONE call replaces 30+. Each command: `{label: "header", command: "..."}`.
2. **FOLLOW-UP**: `ctx_search(queries: ["q1", "q2", ...])` — all questions as array, ONE call (default relevance mode).
3. **PROCESSING**: `ctx_execute(language, code)` | `ctx_execute_file(path, language, code)` — sandbox, only stdout enters context.
4. **WEB**: `ctx_fetch_and_index(url, source)` then `ctx_search(queries)` — raw HTML never enters context.
5. **INDEX**: `ctx_index(content, source)` — store in FTS5 for later search.

## Parallel I/O batches

For multi-URL fetches or multi-API calls, **always** include `concurrency: N` (1-8):

- `ctx_batch_execute(commands: [3+ network commands], concurrency: 5)` — gh, curl, dig, docker inspect, multi-region cloud queries
- `ctx_fetch_and_index(requests: [{url, source}, ...], concurrency: 5)` — multi-URL batch fetch

**Use concurrency 4-8** for I/O-bound work (network calls, API queries). **Keep concurrency 1** for CPU-bound (npm test, build, lint) or commands sharing state (ports, lock files, same-repo writes).

GitHub API rate-limit: cap at 4 for `gh` calls.

## Subagent routing

Routing block auto-injected into subagent prompts. Bash-type subagents upgraded to general-purpose. No manual instruction needed.

## Output

Write artifacts to FILES — never inline. Return: file path + 1-line description.
Descriptive source labels for `ctx_search(source: "label")`.

## Session Continuity

Skills, roles, and decisions persist for the entire session. Do not abandon them as the conversation grows.

## Memory

Session history is persistent and searchable. On resume, search BEFORE asking the user:

| Need | Command |
|------|---------|
| What were we working on? | `ctx_search(queries: ["summary"], source: "compaction", sort: "timeline")` |
| What was the first request? | `ctx_search(queries: ["prompt"], source: "user-prompt", sort: "timeline")` |
| What did we decide? | `ctx_search(queries: ["decision"], source: "decision", sort: "timeline")` |
| What NOT to repeat? | `ctx_search(queries: ["rejected"], source: "rejected-approach")` |
| What constraints exist? | `ctx_search(queries: ["constraint"], source: "constraint")` |

DO NOT ask "what were we working on?" — SEARCH FIRST.
If search returns 0 results, proceed as a fresh session.

## ctx commands

| Command | Action |
|---------|--------|
| `ctx stats` | Call `ctx_stats` MCP tool, display full output verbatim |
| `ctx doctor` | Call `ctx_doctor` MCP tool, run returned shell command, display as checklist |
| `ctx upgrade` | Call `ctx_upgrade` MCP tool, run returned shell command, display as checklist |
| `ctx purge` | Call `ctx_purge` MCP tool with confirm: true. Warns before wiping knowledge base. |

After /clear or /compact: knowledge base and session stats preserved. Use `ctx purge` to start fresh.

## Release

Alpha install and release details: `docs/ALPHA.md`. Commands, run from `electron/`:

```bash
pnpm run release:macos                      # engine build + Forge make (DMG/ZIP)
scripts/smoke-macos.sh out/Bistro-darwin-arm64/Bistro.app   # launch and check the engine's /ws
pnpm run release:win                        # Windows ZIP; run on Windows (the CGO engine builds for the host only)
```

Tagging `vX.Y.Z` (matching `electron/package.json`) runs `.github/workflows/release.yml`: security gates, then build, smoke, and a draft pre-release. Signing runs only when the `MACOS_*` / `APPLE_*` secrets exist.
