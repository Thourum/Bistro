# Bistro agent guide

## What & why

**Bistro** is the keyboard-first code review client for the AI-PR era: a local-first desktop GitHub PR review app. Review PRs at the speed of thought — no browser, no mouse.

Bistro runs as one Wails v3 process, not a client/server pair. SQLite is a local cache, not a shared database. When no `GITHUB_CLIENT_SECRET` is configured (packaged builds), the engine exchanges the OAuth code through `https://get-bistro.com/api/oauth/token`, which holds the secret. Sync and review actions go directly to GitHub; optional BYOK classification and opt-in telemetry are separate outbound integrations.

Authoritative references (read when relevant, don't duplicate here):

- Product overview: [README.md](README.md)
- Build, configuration, and platform prerequisites: [DEVELOPMENT.md](DEVELOPMENT.md)
- Go package map: [docs/go-project-overview.md](docs/go-project-overview.md)
- Roadmap / v1.0 status: [docs/ROADMAP.md](docs/ROADMAP.md)
- Design system: [docs/DESIGN.md](docs/DESIGN.md); mockups in `docs/design/stitch_terminal_authentic_prd/`
- User-facing documentation: `website/src/pages/docs/`
- Architecture decisions: `docs/adr/`

If you change structure, commands, or conventions, update this guide in the same change. `CLAUDE.md` is a symlink to `AGENTS.md`; edit only `AGENTS.md` and keep the symlink.

## Architecture

- `main.go` constructs the Wails application, embeds `frontend/dist/`, wires services, settings, events, and telemetry, and starts the engine. `window.go` owns the native window.
- `internal/engine` owns SQLite writes, sync, local diffs, and offline action replay. There is no WebSocket transport or standalone engine process.
- `internal/services` exposes one service per domain with typed context-aware methods. `ServiceError` codes are `not_found`, `unauthenticated`, `rate_limited`, `invalid`, `cancelled`, `timeout`, and `internal`; optional `Reason` identifies domain failures. `guard` / `guardErr` normalize errors and recover/report panics. `events.go` registers typed Wails events; `servicestest.Recorder` captures them in behavior tests.
- `internal/shell` owns atomic JSON settings, external-edit polling, editor launching, platform operations, Sentry, PostHog, and the updater. `services.Shell` validates external URLs and editor requests before delegating to the platform.
- `frontend/` is the React/Vite renderer. Generated TypeScript models and services in `frontend/bindings/` are committed output, not hand-authored code.

Wails enforces single-instance behavior and focuses the existing window on a second launch. `engine.New(Options)` constructs the engine; its emitter and panic reporter are set before `Start(ctx)` opens/migrates SQLite, restores keychain credentials, builds services, and launches supervised loops. `ShouldQuit` cancels and drains shell-owned initial/periodic updater checks off the native thread, then retries quit before native cleanup. `OnShutdown` runs once-guarded teardown (watcher, engine, telemetry, Sentry, log, smoke directory); the deferred fallback handles startup errors and platforms whose run loop returns. Settings commits deliver listeners in order outside the snapshot mutex; renderer writes queue leaf-only section patches and resets. `--smoke` uses a temporary data directory, disables sync, replay, telemetry, and updates, verifies the native window/runtime, and exits within a bounded time. Only OAuth retains a temporary loopback callback listener.

Within `internal/engine`, `app.go` owns Start/Stop, `options.go` construction/defaults, `supervise.go` panic restart/backoff, and `services.go` the retained service set. `provider.go`, `config.go`, `llm.go`, `expertise.go`, `bridge.go`, and `repo_location.go` own domain wiring. Auth uses blocking `Auth.Start` with context cancellation. `App.ApplySyncSettings` clamps intervals to 30–3600 seconds; `App.ApplyClassificationSettings` shares validated service state.

`internal/tray` owns the pure sync-status view model and Wails adapter. In `package main`, `background.go` wires tray actions and ordered window/Dock visibility; `appmenu.go` owns the native app menu. The engine's `syncStatusTracker` sits behind the emitter, publishes `sync:status`, and supplies the `Sync.Status` snapshot and tray listener. Native menu clicks emit `menu:action`; the renderer reports enabled actions through `Shell.SetMenuActions`. Background mode is on only when the tray is ready (created on macOS or Windows), never on Linux or under `--smoke`; see [DEVELOPMENT.md](DEVELOPMENT.md#background-mode) for close and quit behavior.

Within `internal/git`, `Client` owns clone/fetch/checkout; `anchor` resolves comment anchors, `diff` generates diffs/interdiffs and patch fingerprints, `remote` parses canonical identities, and `repolock` owns shared per-repo locks. Interdiff takes the same `*repolock.Locks` as clone/fetch, not a `git.Client`. `clonecache` depends on `remote`, never the parent package; `repolock` depends on `clonecache` only for the `BISTRO_REPO_DIR` override check. Shared on-disk commit fixtures live in `git/gittest`.

Independent SQLite stores live in `internal/store/currentuser`, `dismissal`, `prdiff`, `repolocation`, and `reviewdraft`; the offline queue lives in `store/actionqueue`. The root `store` package retains the DB handle, migrations, and DB-bound queries. Subpackages may import `store` and `store/db`, never the reverse. Expertise persistence belongs to `internal/expertise`.

`internal/prsync` orchestrates PR sync. `prsync/workerpool` bounds concurrent fetches; `prsync/writer` owns fetched results and SQLite batch writes. Neither imports the parent package. Go tests outside `store` share migrated in-memory DBs via `internal/store/storetest.Open(t)`; version-limited tests use `store.MigrationFS()` with goose.

## Tech stack

- **Go 1.26.1**, module `github.com/itterno/bistro` — go-git, go-github, mattn/go-sqlite3, go-keyring, x/oauth2, sentry-go, posthog-go.
- **Wails v3.0.0** and **`@wailsio/runtime` 3.0.0** — WKWebView on macOS, WebView2 on Windows, WebKitGTK on Linux.
- **React 19 + TypeScript + Vite** — TanStack Query / Hotkeys / Virtual / Store, Radix UI, Tailwind, Biome, Vitest.
- **pnpm** — run frontend commands from `frontend/`; the lockfile is repo-root `pnpm-lock.yaml`. `pnpm-workspace.yaml` covers `frontend/` and `website/`. Its `minimumReleaseAge` blocks versions younger than 7 days. The sole exemption is exactly `'@wailsio/runtime@3.0.0'`, approved by the user on 2026-10-29 to match the pinned Go runtime; do not broaden it. Read versions from `go.mod` and `frontend/package.json`.

## Conventions

- **Frontend TS/JS error handling uses `@thourum/fault`, never `try`/`catch`.** Chain `Result<T, Fault>` / `ResultAsync<T, Fault>`, narrow by `fault.tag`, and use `fault.message` only at display/log boundaries. Go uses its existing error style. The only exception is throwaway context-mode sandbox scripts.
- Go owns all SQLite writes; SQLite is a cache with versioned, idempotent, embedded migrations.
- Review drafts belong to the engine. `approve` / `request_changes` absorb current-user drafts in `Orchestrator.ApplyAction`. `Threads.SubmitAction` with action `submit_review` is never queued; failed submissions retain unsubmitted drafts.
- Keyboard bindings are pure config, no handlers: actions (`actionId`, label, activation) in `ACTION_CATALOG` in `frontend/src/utils/keybindings.ts`, keys per navigation style (`vim` / `emacs` / `standard`) in `utils/keymaps.ts` (`KEYMAPS`, `SEQUENCE_TIMEOUT_MS`). `getEffectiveBindings(style, overrides)` resolves user override → style table → unbound; handlers live in `utils/keybindingActions.ts` (line-comment handlers in `utils/lineCommentActions.ts`, draft and resolve handlers in `utils/reviewActions.ts`, onboarding handlers in `utils/onboardingActions.ts`), enable/scope rules in `utils/keybindingActivation.ts`; registration in `hooks/useAppHotkeys.ts`. Don't register hotkeys outside `useAppHotkeys` (textarea-local submit uses `TEXTAREA_SUBMIT_HOTKEY` / `isTextareaSubmit` from the same file). Command-palette entries dispatch the same handler by `actionId` (`getBindingAction`); palette and hotkeys share effective-route activation and the ref-backed action context in `hooks/useKeyActionContext.ts`. Palette-only actions (no key in any style) live in `ACTIONS` without a binding. Entries live in `components/commandPaletteCommands.ts`; `shortcutFor` shows a shortcut only when `isActive` permits the binding on the current route. Settings → Keyboard records overrides with `useHotkeySequenceRecorder`; recording and default resets reject conflicts via `utils/keybindingConflicts.ts`, using the runtime `isBindingEnabled` ownership gates.
- Engine calls use generated `@bindings/services` methods through `call()` / `callWrite()` in `frontend/src/services/engine/call.ts`. Never call bindings directly from components. Reads use `call`; mutations and external side effects use `callWrite`, which refuses writes under `?bench_keys=1`.
- Faults carry `code` / `reason` context. Branch on `faultReason`, never message text. Calls default to a 30-second timeout and cancel the Wails promise on expiry; `Auth.Start` and dev-folder scans disable that timeout.
- Push events are registered in `internal/services/events.go` and handled by the exhaustive table in `frontend/src/services/events/index.ts`. `useEngine` owns subscriptions and once-only startup checks.
- Native menu action items dispatch by `actionId`, with no native accelerators (built-in roles keep theirs). `useMenuActions` / `utils/menuActions.ts` share the hotkey and palette handlers and report enablement from `isActive` through `Shell.SetMenuActions`.
- Never call the Wails Dock service on the main thread: `HideAppIcon` / `ShowAppIcon` synchronously dispatch there on macOS. Use the ordered visibility worker in `background.go`.
- Generated Wails models type engine responses/events. Use zod only at genuinely external edges: settings-file leaves, URL search params, bench collector input, DOM events, and the untyped RuntimeError cause in `call.ts`; never replace validation with `as` casts.
- After changing a bound service method or event shape, run `wails3 task generate` and commit `frontend/bindings/`. Never hand-edit generated bindings or `internal/store/db/`.
- Hooks own React state/effects and fetching lifecycles; requests and appStore writes live in `frontend/src/services/`. Call services directly, not hooks created solely for `useCallback`.
- Large pages keep feature-local pieces beside them: `pages/settings/*Section.tsx`, `use<Page>Data.ts` / `use<Page>Actions.ts`, and `*Sections.tsx` / `*Chrome.tsx`.
- Onboarding / first-run rail pure models live in `utils/onboardingRepos.ts`, `utils/onboardingSandbox.ts`, and `utils/firstRunRail.ts`; binding calls and store writes live in `services/onboardingSync.ts` and `services/firstRunRail.ts`. Engine methods live in `internal/services/{review_queue,dev_folder_scan,repos}.go`: `ReviewQueue.Get` uses lowercase `identity.ID()` repo IDs; a newer `DevFolderScan.Scan` cancels its predecessor, and Stop cancels the call context; `Repos.List` supports `cache_only`. Binding-owned UI uses `useBindingEnabled`.
- Read the Go module path from `go.mod`; do not assume it from memory.

## How to work

There is no Makefile. The root `Taskfile.yml` is authoritative; the pinned Wails CLI includes its task runner (no separate `task` binary):

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
pnpm install
wails3 task dev
wails3 task generate
wails3 task build
wails3 task package
wails3 task package:darwin
wails3 task package:windows
wails3 task package:linux
wails3 task setup:docker
```

Run these from the repository root; packaging requires each platform's tools (see [DEVELOPMENT.md](DEVELOPMENT.md#packaging)). Dev uses isolated `.dev-data/`. Runtime configuration is `BISTRO_DATA_DIR`, `GITHUB_CLIENT_ID` / `GITHUB_CLIENT_SECRET`, and `BISTRO_LOG_LEVEL`; build-time `BISTRO_VERSION` stamps the version. Never commit secrets.

For SQL changes, edit `internal/store/queries/*.sql`, then run `wails3 task generate` (bindings + sqlc), or `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate` from the root. Generated models must match migrations.

Verify before declaring done:

```bash
go vet ./... && go test -race ./...
cd frontend && pnpm type-check && pnpm lint && pnpm test
```

Enable the existing pre-commit hook with `git config core.hooksPath .githooks`. It blocks staged authored files over 5,000 estimated tokens (bytes/4); exemptions include lockfiles, sqlc output, images, and generated-file headers. There is no bindings-directory skip rule: committed `frontend/bindings/` files stay under the limit and carry DO NOT EDIT headers. Never bypass the hook.

ADR 0001's bench harness remains a historical Electron record: `BISTRO_SLIM=1 pnpm run release:macos`, `--quit-after-ready`, and its Electron paths are not current build commands. The retained renderer collector loads only with `bench_collector` in the URL; seeding uses `cmd/bistro-benchseed` and pinned PRs. S6 keyboard drivers require passive permission/layout preflight and an explicit bounded `--limit` or `--full`; never inject keys while preflight is blocked. Do not rewrite historical harness evidence in `docs/adr/data/`.

If a command is unclear, read `Taskfile.yml` and `frontend/package.json` rather than inventing a workflow.

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

Install/update instructions live in [docs/ALPHA.md](docs/ALPHA.md). `.github/workflows/release.yml` builds native Wails bundles on macOS, Windows, and Linux, runs `scripts/smoke.sh`, and creates a draft pre-release on `v*` tags. Update archives are `bistro-darwin-arm64.zip`, `bistro-windows-amd64.zip`, and `bistro-linux-amd64.tar.gz`; first-install artifacts are `bistro-darwin-arm64-installer.dmg` and `bistro-windows-amd64-installer.exe`. All five assets are covered by `SHA256SUMS`.

Local macOS verification: `BISTRO_VERSION=0.0.0-local wails3 task package:darwin && scripts/smoke.sh bin/Bistro.app`. Smoke isolates data and disables sync; Linux needs `xvfb`, macOS needs `timeout` or coreutils' `gtimeout`. Signing/notarization uses `MACOS_*` / `APPLE_*` secrets when configured. The built-in updater checks GitHub pre-releases on launch and every six hours, verifies `SHA256SUMS`, and is disabled in dev/untagged builds. Do not push release tags or trigger publishing without explicit user permission.
