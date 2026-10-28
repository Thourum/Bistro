# ADR 0001: Electron vs Wails v3, and WebSocket vs Wails bindings

- **Status:** In progress. Gate 0 failed, the owner overrode it (see Amendments), and Gate 1 thresholds are pre-registered below.
- **Date:** 2026-10-27
- **Gate 0 evidence commit:** `c71ea2e` (the tree the inventories were collected from)
- **Reproduce:** Gate 0: `docs/adr/data/0001/gate0-collect.sh` (read-only). Gate 1: `docs/adr/data/0001/harness/gate1.sh` (see its README).

## Amendments

- **2026-10-27, Gate 0 override.** The owner chose to continue past the failed Gate 0: "it just means migration wouldn't impact anyone other than myself." Gates 1–5 now run with the owner as the only affected user. Gate 0's verdict and evidence stand unchanged below. The pre-registered thresholds in Gate 1 are judged against the owner's workload (the measured fixtures), not against a user population.
- **2026-10-28, pre-registration hash.** The thresholds under "Pre-registration (Gate 1.4)" were committed in `755947e`, before any WebKit or Wails measurement. They haven't changed since; check with `git diff 755947e -- docs/adr/0001-electron-vs-wails.md`.
- **2026-10-28, S7 fails at Gate 3, and the owner overrides the stop.** The median time to a first maintainer response over the last 100 Wails issues is ≥ 18.89 days, against a threshold of 14 (`data/0001/gate3/s7-health.md`). Under rule S, the shell verdict is therefore **no-go**, and no later gate can reverse it. The owner chose to run the remaining gates anyway, to have the evidence. Later results are recorded as evidence only; they don't reopen the S7 failure.

## Question

Two separate decisions, with separate thresholds and verdicts:

1. **Shell:** should Bistro move from Electron to Wails v3?
2. **Transport:** should the renderer stop using the localhost WebSocket to `bistro-sync` and use Wails bindings and events instead?

The method was gated: stop at the first gate that fails. Gate 0 asks whether a user-visible problem exists, and what fixing it would cost.

## Verdicts

| Decision | Verdict | Rule applied |
|---|---|---|
| Shell (Electron → Wails v3) | **No-go.** S7 (upstream health) fails at Gate 3. Gate 0 was also no-go, and the owner overrode it. Gates 4–5 run as evidence only. | Gate 0.5, then rule S |
| Transport (WebSocket → bindings) | **(a) Keep the WebSocket.** The shell is no-go, so rule T isn't evaluated. Gate 3's source trace (T1, T2, T6) is kept as evidence. | Rule T |

## Pre-registration (Gate 1.4)

These thresholds were committed before any WebKit or Wails measurement, and before the full Gate 1 Electron baseline existed; only n=1 harness smoke runs had been made. The commit that introduces this section is the pre-registration. Changes after it go in a dated amendment that gives the reason; the thresholds below are never edited in place.

**Comparator.** All shell comparisons are against **slim Electron** (`BISTRO_SLIM=1`), measured on the same machine in the same conditions, with the same harness (`docs/adr/data/0001/harness/`) and the same fixtures. There are four fixtures, with the real PRs pinned to their merged heads:
- vendoring: `kubernetes/kubernetes#143358` @ `09048f5c`
- generated code: `googleapis/google-cloud-go#20526` @ `de1e8de4`
- synthetic: 1000 files × 10 lines
- synthetic: 1 file × 10,000 lines

The WKWebView host (Gate 2.3) is checked against S1 first, as an early stop; the Wails spike (Gate 4) is checked against everything.

**Why the bar is high.** The owner is the only affected user, on a 36 GiB machine, so size and memory matter little to them. On the other side is the migration cost from Gate 0:
- ≈3.1k lines of main-process code
- 21 IPC channels
- continuity for the updater, keychain and settings

A move is justified only by a large size and memory gain with **no regression** where the owner works: scrolling large diffs, startup and the keyboard.

### Shell thresholds (all must pass)

- **S1 Large-diff frame times.** Checked per fixture, pooling all frames over ≥10 runs, on the current macOS **and** on macOS 13, the oldest supported version (Gate 2.3, `tart`). Every one of these must hold:
  - pooled p95 ≤ slim p95 + 2.0 ms
  - pooled p99 ≤ max(slim p99 × 1.10, slim p99 + 3.0 ms)
  - late-frame share (> 1.5 × the run's measured refresh interval) ≤ slim share + 2.0 percentage points
  - median of per-run p95 ≤ slim median per-run p95 + 2.0 ms

  The display here runs at 60 Hz. The 120 Hz column the harness reports is informational only and can't decide anything on this hardware.
- **S2 Startup.** Launch until the first PR list has rendered, then exit; n = 30 and the same seed for both. The candidate must reach p50 ≤ slim p50 × 1.05 and p95 ≤ slim p95 × 1.10. No startup gain is required, but no regression is allowed.
- **S3 Idle memory.** The `footprint` sum over the whole process tree; for WebKit that includes every `com.apple.WebKit.*` XPC process plus `bistro-sync`; n ≥ 5. The candidate must reach median ≤ 0.70 × slim median **and** max < slim min, so the two distributions don't overlap.
- **S4 Large-diff memory.** Measured after the largest real fixture: median ≤ 0.80 × slim median.
- **S5 Size.** Installed `.app` ≤ 0.40 × slim, **and** DMG ≤ 0.40 × slim.
- **S6 Keybindings.** A *cell* is one binding × navigation style × layout (US / DE / AZERTY / Dvorak) × injection (`osascript` / `CGEventPost`) × focus (outside / inside a comment textarea) × IME (inactive / active). A cell fails when the candidate's outcome (which `actionId` fired or didn't, and for typing the resulting text) differs from Electron's in the same cell. Allowed failing share per class:

  | Class | Allowed |
  |---|---|
  | plain keys (no modifier), outside text fields | 0% |
  | Cmd combinations | 0% |
  | Ctrl combinations | 0% |
  | Option combinations, US layout | 0% |
  | Option combinations, DE / AZERTY / Dvorak | ≤ 5%, and only if each failing binding can be rebound in Settings → Keyboard (workaround recorded) |
  | multi-key sequences | 0% |
  | inside text fields (any modifier) | 0% |
  | IME composition active | 0% (any binding firing mid-composition is a failure in either shell) |

  Regardless of share, any failure that disables a core review action (approve, request changes, submit or save a draft, next/previous file or hunk) is a blocker.
- **S7 Upstream health** at the pinned Wails v3 version (Gate 3.4). All of these must hold:
  - the release is at least a beta/RC, or an alpha with a published API-stability statement
  - ≥ 6 releases (pre-releases included) in the last 12 months
  - the top committer has ≤ 70% of the non-bot commits in the last 12 months
  - the median time to first maintainer response over the last 100 issues is ≤ 14 days
  - there is no open macOS keyboard issue in an S6 class without a workaround
- **S8 Continuity** (Gate 5). Each of these items has either a demonstrated pass or a named bridge release:
  - in-place update (Squirrel.Mac to the Wails bundle)
  - the keychain token and its access list
  - the `electron-store` settings
  - the SQLite DB and its migration version
  - queued offline actions replaying after the upgrade
  - `codesign --verify --deep --strict`, notarization and `spctl`

**Shell rule:** **go** iff S1–S8 all pass. Any single failure is **no-go**, and gates stop at the first failure.

### Transport thresholds

These are evaluated only if the shell verdict is go. Three options:
- (a) keep the WebSocket with `bistro-sync` as a separate process
- (b) Wails bindings and events, with the engine embedded in the app process
- (c) a hybrid: the WebSocket stays the data protocol, and bindings handle only shell features

- **T1 Round trip.** Over the request mix — `REQUEST_DIFF` of the largest real fixture, `REQUEST_REPO_LIST` and `REQUEST_PR_LIST`, each with n ≥ 200 — (b) must reach p50 ≤ 0.5 × (a) p50 **and** p99 ≤ (a) p99.
- **T2 Push burst** of 1000 events: (b) throughput ≥ (a), and p99 delivery latency ≤ (a). This is a no-regression check.
- **T3 Startup.** (b) saves ≥ 150 ms at p50 against (a), with no p95 regression.
- **T4 Memory.** (b) median idle tree sum ≤ (a) − 30 MiB.
- **T5 Failure isolation.** Inject a Go panic and a handler deadlock. (b) must keep unsubmitted drafts, show an error state within 5 s, and offer recovery. A silent whole-app crash or a frozen window fails.
- **T6 Security.** (b)'s bindings must be callable only from the app's own origin. The credit for removing the socket counts against (a) *with* a per-launch token (Follow-up 2), not against today's gap that accepts a missing `Origin`.
- **T7 Equivalence.** The chosen option passes 100% of the contract suite: request/response and event shapes, `SCAN_CANCEL`/`AUTH_CANCEL`, and offline queue replay. zod validation stays at the edge.

**Transport rule:** choose **(b)** iff T1 ∧ (T3 ∨ T4) ∧ T5 ∧ T6 ∧ T7 pass and T2 doesn't regress. Otherwise choose **(c)** if it passes T5–T7, which is the default. If the shell verdict is no-go, the transport stays **(a)**.

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

## Status of later gates

Gate 0 originally stopped here. After the override (see Amendments), the following was done:
- Gate 1 tooling exists and was verified with n=1 smoke runs only:
  - `--quit-after-ready` and the bench module (`electron/src/renderer/bench/`)
  - the slim build
  - `go/cmd/bistro-benchseed`
  - the harness
- Thresholds are pre-registered above.

Still to do:
- **The full Gate 1 baseline.** It's blocked: the engine's keychain token returns HTTP 401, so the two pinned GitHub PRs can't be seeded until the owner signs in again.
- **Gates 2–5.**
- **No second machine:** none is available, and `tart` isn't installed.
- **True cold starts:** they need `sudo purge` without a password; until then, startup runs are warm and labelled as such.
- **The blind alpha-tester session.**

Only the throwaway spike may be migrated.

## Reopening

Reopen at Gate 1, with a new pre-registration commit, when **any** of these holds:

- An issue or alpha report names size, memory, startup or diff-scrolling performance as a reason to stop using Bistro, or to not install it.
- Telemetry from a distributed build shows a crash class or OS-version cluster attributable to Chromium/Electron. Run `gate0-collect.sh` with PostHog and Sentry credentials set; the raw JSON lands beside the script.
- A distribution constraint appears that Electron can't meet (for example, a hard download-size cap).

## Follow-ups (independent of this ADR)

1. Configure Electron fuses (`@electron-forge/plugin-fuses`), and drop unused fonts and dependencies. This is cheap, and it's the "slim Electron" baseline any future comparison is required to use.
2. Add a per-launch WS auth token so a missing `Origin` isn't trusted on its own (see 0.4).
3. Ship a tagged alpha with telemetry enabled, so a future Gate 0 has an n.
