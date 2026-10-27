# ADR 0001: Electron vs Wails v3, and WebSocket vs Wails bindings

- **Status:** Decided, no-go on both. Stopped at Gate 0.
- **Date:** 2026-10-27
- **Evidence commit:** `c71ea2e` (the tree the inventories were collected from)
- **Reproduce:** `docs/adr/data/0001/gate0-collect.sh` (read-only; rewrites the `*.txt` files beside it)

## Amendments

- **2026-10-27, Gate 0 override.** The owner chose to continue past the failed Gate 0: "it just means migration wouldn't impact anyone other than myself." Gates 1–5 now run with the owner as the only affected user. Gate 0's verdict and evidence stand unchanged below. The pre-registered thresholds in Gate 1 are judged against the owner's workload (the measured fixtures), not against a user population.

## Question

Two separate decisions, with separate thresholds and verdicts:

1. **Shell:** should Bistro move from Electron to Wails v3?
2. **Transport:** should the renderer stop using the localhost WebSocket to `bistro-sync` and use Wails bindings and events instead?

The method was gated: stop at the first gate that fails. Gate 0 asks whether a user-visible problem exists, and what fixing it would cost.

## Verdicts

| Decision | Verdict | Rule applied |
|---|---|---|
| Shell (Electron → Wails v3) | **No-go: no problem to solve** | Gate 0.5: no user-visible problem. |
| Transport (WebSocket → bindings) | **No-go: no problem to solve** | Gate 0.5 for the same reason. Gate 4b depends on a shell go, so it was never reached. |

Gates 1–5 did not run. In particular, **no thresholds were pre-registered.** Gate 1.4 says to commit them before any WebKit measurement. Writing them now, with no baseline and no problem to aim at, would just be a number picked to pass. A reopened decision starts at Gate 1 with a fresh pre-registration commit; see "Reopening".

## Measurement conditions

From `data/0001/env.txt`: macOS 27.2 (26B118), Mac15,10, Apple M3 Max, 36 GiB, on AC power. `pmset` reports no Low Power Mode setting on this machine. Gate 0 involves no timing measurements, so thermal state doesn't matter yet. No second machine (base 8 GB M1) and no `tart` was available; `tart` isn't installed (`env.txt`, `telemetry-access.txt`).

## Gate 0 evidence

### 0.1 Telemetry and crash reports

| Source | Result | n | Raw |
|---|---|---|---|
| PostHog (HogQL) | **Not queried.** No `POSTHOG_PERSONAL_API_KEY` / `POSTHOG_PROJECT_ID` on this machine. The script runs the query once they're set. | unknown | `telemetry-access.txt` |
| Sentry | **Not queried.** No `SENTRY_AUTH_TOKEN`/org/project, and no `sentry-cli`. The script runs the query once they're set. | unknown | `telemetry-access.txt` |
| Local `.ips` (`~/Library/Logs/DiagnosticReports`) | 35 reports, **0** from Bistro, Electron or `bistro-sync` | 0 | `ips.txt` |

There's a structural reason telemetry can't change the answer yet. The repo has **0 releases and 0 tags** (`github.txt`), so no packaged build has been distributed and there is no population whose events or crashes could show a problem. The one installed copy is the developer's own `0.1.0-alpha.1`. **n is too small to support any conclusion about the OS mix or crash classes.** The honest figure is n ≈ 0 outside this machine. Don't read "0 crashes" as "stable". It means "not yet observed".

### 0.2 Complaints about size, memory, startup or performance

`gh issue list --state all` on `Thourum/Bistro` returns **0 issues of any kind**. Searches for size, memory, RAM, startup, slow, performance, lag, freeze, crash, electron, wails and webkit each return 0. Discussions are disabled. The repo has 1 star and 0 forks. No alpha-feedback channel with content exists in the repo either: `docs/COMMUNICATIONS.md` describes the feedback intake *plan*, not received feedback. Raw: `github.txt`.

For scale only (n=1, not a gate metric): the installed alpha is 334 MB on disk. That's 294 MB of Electron framework, a 21 MB `app.asar` and an 18 MB `go-bin` (`installed-size.txt`). It's a known cost of Electron, but no user has reported it as a problem.

### 0.3 Shell migration cost (inventory)

Raw: `shell-inventory.txt`.

- **13 files** in `electron/src` import Electron or shell-coupled packages: `electron` ×8, `electron-log` ×9, `@sentry/electron/{main,renderer}` ×4, `posthog-node` ×2, `electron-store` ×1, `update-electron-app` ×1.
- **21 IPC channels** (`engine:*` ×7, `settings:*` ×5, `go:command`, `go:event`, `sync:action`, `db:query`, `fs:read-patch`, `open-external`, `open-in-editor`, `app:quit`, `dialog:pick-folder`) over one `contextBridge` surface (`window.electron`). The renderer calls it from 9 distinct APIs: `pickFolder`, `settings.{get,set,reset,openFile}`, `quitApp`, `openInEditor`, `openExternal`, `on`.
- **3,077 lines** of non-test main-process TypeScript. Each of these would need a Go or Wails equivalent:
  - engine spawn and the ready handshake (`go-spawn.ts`)
  - window lifecycle (`window.ts`)
  - settings and their on-disk format (`settings.ts` / `electron-store`)
  - updater (`updater.ts` / Squirrel.Mac via `update-electron-app`)
  - crash reporting (`sentry.ts`, `crashReport.ts`)
  - telemetry (`telemetry.ts`)
  - editor launching (`editorLaunch.ts`, `editorRequest.ts`, `editorTarget.ts`)
  - the main process's own engine client (`websocket-client.ts`)
- Packaging is already partly slimmed: `asar: true`, the 54 unused locale packs are stripped, and the DMG uses LZMA. **Electron fuses aren't configured** (`forge.config.ts`). That slimming would come before any Wails comparison (Gate 1.2), and it's worth doing for its own sake.

### 0.4 Transport surface (inventory)

Raw: `transport-inventory.txt`.

- **Push events:** 68 handler keys in the exhaustive table in `services/ws/events/index.ts`.
- **Engine message types:** 24 handler files in `go/internal/server/handler/`. There are 101 distinct uppercase string literals in `go/internal/server`, which overcounts a little (enum values like `LEFT`/`RIGHT`/`APPROVE` appear too).
- **Renderer call sites:** 11 direct `wsClient.request`/`send` sites, plus the services that wrap them.
- **Request IDs and cancellation:**
  - Every engine handler logs `request_id` = `msg.ID`.
  - `SCAN_DEV_FOLDER` → `SCAN_CANCEL {scan_id}` uses the id from `wsClient.request(..., { onSent })` (`services/onboardingSync.ts`).
  - `AUTH_CANCEL` is sent on sign-in abort and again on reconnect when `authInterrupted` is set (`services/signIn.ts`, `services/ws/connection.ts:202`).
  - `useExpertise` guards stale responses with a latest-request-id ref.
- **Consumers besides the renderer:**
  1. The **Electron main process itself**: `websocket-client.ts` relays `go:command` / `sync:action` / `go:event` over IPC.
  2. `electron/scripts/smoke-macos.sh`, the release smoke test that checks `/ws`.
  3. The Go WS tests (`go/internal/ws/*_test.go`).
  - The Bistro Team server (`~/dev/bistro-server`, excluding the `upstream/` submodule) has **no** WS consumer.
- **Current security posture:** the server binds loopback only (`go/internal/server/wsaddr.go`). `CheckOrigin: isLocalOrigin` (`go/internal/ws/server.go:119`) accepts `file://`, loopback and `localhost` origins, and rejects malformed or lookalike origins. That rejection covers DNS rebinding through a browser. **It also accepts an empty `Origin`**, for non-browser clients. So any local process running as the user can connect without credentials and drive the engine. That's a real gap. It's independent of this decision, and a per-launch token passed to the renderer would close it without Wails. It's listed under follow-ups, not counted as a reason to migrate.

### 0.5 Gate result

No user-visible problem exists: 0 complaints, 0 crash reports, 0 distributed builds. **Gate 0 fails, and both decisions are no-go: no problem to solve.** The cost side (≈3.1k lines of main-process code, 21 IPC channels, updater/settings/keychain continuity, 68 push events and the cancellation semantics) is recorded so that a future reopening doesn't have to rediscover it.

## Not done (by the gate rule)

Gate 1 (baseline, slim build, `--quit-after-ready`, pre-registration), Gate 2 (WebKit/WKWebView), Gate 3 (upstream reading and Wails health), Gate 4/4b (spikes, the keyboard matrix, the transport options), Gate 5 (updater, install-over, notarization), the second-machine replication, and the blind alpha-tester session. No code was migrated, and no `--quit-after-ready` flag was added.

## Reopening

Reopen at Gate 1, with a new pre-registration commit, when **any** of these holds:

- An issue or alpha report names size, memory, startup or diff-scrolling performance as a reason to stop using Bistro, or to not install it.
- Telemetry from a distributed build shows a crash class or OS-version cluster attributable to Chromium/Electron. Run `gate0-collect.sh` with PostHog and Sentry credentials set; the raw JSON lands beside the script.
- A distribution constraint appears that Electron can't meet (for example, a hard download-size cap).

## Follow-ups (independent of this ADR)

1. Configure Electron fuses (`@electron-forge/plugin-fuses`), and drop unused fonts and dependencies. This is cheap, and it's the "slim Electron" baseline any future comparison is required to use.
2. Add a per-launch WS auth token so a missing `Origin` isn't trusted on its own (see 0.4).
3. Ship a tagged alpha with telemetry enabled, so a future Gate 0 has an n.
