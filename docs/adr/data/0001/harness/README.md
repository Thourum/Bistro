# ADR 0001 measurement drivers

Node 24, no npm dependencies. macOS measurement requires `hyperfine`, `footprint`, and an already packaged build with the Task 1 bench hooks. Seeding additionally requires Go and `gh`, both authenticated through their normal stores: log in to Bistro for the engine keychain, and to `gh` for read-only head checks. Never pass tokens to these scripts.

From the repository root, one-command reproduction for an **already built** app:

```bash
docs/adr/data/0001/harness/gate1.sh \
  --app electron/out/Bistro-darwin-arm64/Bistro.app \
  --label baseline --out docs/adr/data/0001/gate1/baseline \
  --dmg electron/out/make/Bistro.dmg
```

Use the actual DMG path produced by Forge, or omit `--dmg` and report download size as unmeasured. Build/package instructions are in `docs/ALPHA.md`; the wrapper builds no app. It only builds the standalone Go tools if the seed is missing. Reuse the same seed for baseline and slim; change `--app`, `--label`, and `--out`. Output must be empty. `--runs 1` is a smoke run, **not** gate-quality evidence.

Default counts: 30 timed startup runs plus one warmup; 5 idle-memory samples; 10 scrolling runs per fixture, with post-diff memory on the first 5. `--runs N` overrides all three counts. Four fixtures when Task 4 exists: the two pinned merged GitHub PRs and the two synthetic shapes. A missing synthetic seeder is warned about and marked incomplete, not silently treated as complete evidence.

## Individual steps

```bash
node docs/adr/data/0001/harness/seed.mjs --out /tmp/bistro-bench-seed
node docs/adr/data/0001/harness/run-electron.mjs --app /path/Bistro.app \
  --label baseline --seed /tmp/bistro-bench-seed --out /tmp/bistro-gate1-baseline
node docs/adr/data/0001/harness/stats.mjs /tmp/bistro-gate1-baseline
node --test docs/adr/data/0001/harness/harness.test.mjs
# Apply S1/S3/S4/S5 to a WebKit candidate (Wails: add its .app KiB and DMG bytes):
node docs/adr/data/0001/harness/evaluate-shell.mjs docs/adr/data/0001/gate1/slim docs/adr/data/0001/gate4/wails-full 70624 28023742
```

`BISTRO_BENCH_SEED=/path` overrides the wrapper's `/tmp/bistro-bench-seed`. Seed accepts `--engine /path/bistro-sync` and `--timeout-minutes 30` (minimum 10). Without `--engine` it builds from the checkout. Pinned head prefixes are checked through read-only GitHub API calls before `REQUEST_PR_SYNC`; each diff must have nonempty file rows, nonempty text, and matching addition/deletion row totals. Rerun seed after Task 4 becomes available. A failed seed has no completion manifest; engine logs remain for diagnosis. Seed DBs contain fixture metadata/comments: keep raw artifacts private until inspected.

## Isolation and timing

Each run gets fresh `sync.db`, optional SQLite WAL, and `settings.json`; profiles are removed after the process exits, logs are retained. Clone caches/binaries/manifests are not copied into the timed profile. `BISTRO_REPO_DIR` always points to the seed's dedicated cache. Cached diffs need no clone during measurements. `WS_PORT=0` forces a driver-owned engine instead of connecting to an unrelated listener on 8080. The driver refuses memory samples without an engine in the process tree. Settings disable periodic sync, onboarding/tour, notifications, and telemetry. Auto-updates are disabled in the environment. No GitHub write action is requested; startup can still perform ordinary provider reads.

Hyperfine's prepare step resets the profile outside the timed command. It adds `sudo -n purge` only when `sudo -n true` succeeds; otherwise `conditions.txt` explicitly says **warm, no purge**. No password prompt or sudo setup occurs. Startup timing includes shell/app launch through ready-triggered process exit, not only renderer `performance.now()`. A timeout or renderer `error` fails the driver. `completed.json` is written only after all phases finish; stats verifies this marker and every expected fixture/run/memory sample against `run-manifest.json`, rejecting interrupted or incomplete output. Preparation requires a driver-created ownership marker and resolved paths; it never accepts a generic `user-data` directory. Interrupts clean up the owned process group, including descendants after root exit.

The collector is importable (`createCollector({file})`) or runnable as `collector.mjs --out events.jsonl`. It binds only `127.0.0.1:0`, prints its URL, accepts `POST /event` with text/plain JSON, answers OPTIONS, and returns CORS `*`. `waitFor(eventName)` handles both future and already-received events. Every receipt includes `receivedAt` and collector-local `receivedPerfMs`; neither is comparable to renderer `t` without a time-origin conversion. This server is for temporary local measurement, not general application transport.

## Raw output and interpretation

- `seed-manifest.json`: PR IDs, pinned full head SHAs, diff rows/additions/deletions/bytes, checkout engine revision, synthetic completeness.
- `run-manifest.json`: build path/label, seed identity, runtime, requested counts.
- `sizes.txt` / `sizes.json`: allocated `.app` size from `du -sk`, executable stat bytes, optional DMG stat bytes and allocated size. These are different metrics, deliberately labelled.
- `conditions.txt`: macOS, hardware model, battery/AC, power settings including low-power mode, thermals, display report including refresh/scaling information where the OS exposes it. Measured idle-rAF refresh is retained per fixture/run as the timing reference; a missing display refresh in `system_profiler` is not fabricated.
- `startup.json`: hyperfine raw times in seconds; `startup/` retains events, command and logs.
- `idle/run-N/` and `frames/<fixture>/run-N/`: raw events/logs, process tree, raw footprint JSON/text, and normalized per-process byte samples. Memory is the **sum of per-process footprint**; shared allocations may be counted in multiple processes, not a unique physical-memory total.
- `summary.json` / `summary.md`: startup p50/p95/p99 and seeded 1,000-resample median bootstrap 95% CI; pooled frame p50/p95/p99, per-run p95 min/median/max, run/frame counts; per-run memory sum min/median/max. Quantiles use linear interpolation at `(n−1)×q`, never means.

Dropped-frame share is the fraction of observed deltas greater than **1.5× that run's measured refreshMs**, plus a separately labelled **120 Hz target (8.33 ms ×1.5 = 12.50 ms)**. It is a late-frame share, not the number of missed refresh slots. Pooled tails weight runs by their frame count; per-run p95 distributions expose run variability. Small samples do not justify precise tail or population claims.

Protocol and fixtures are defined in `docs/superpowers/plans/2026-10-27-adr0001-gate1-harness.md`; the decision and thresholds belong in `docs/adr/0001-electron-vs-wails.md`.

## Gate 2: exact production renderer

Extract the already packaged slim renderer using the repository's installed `@electron/asar` (no rebuild). Run from the repository root:

```bash
node --input-type=module -e 'import {extractAll} from "@electron/asar"; extractAll("electron/out/slim/Bistro-darwin-arm64/Bistro.app/Contents/Resources/app.asar", "docs/adr/data/0001/gate2/extracted-smoke")'
node docs/adr/data/0001/harness/serve-renderer.mjs \
  --root docs/adr/data/0001/gate2/extracted-smoke/.vite/renderer/main_window
```

`serve-renderer` binds an ephemeral localhost port, prints its URL, serves the built bundle with JS/wasm MIME types and SPA fallback, and rejects traversal, symlink escapes and non-read requests. Browser drivers call its importable `serveRenderer({root})` themselves; do not leave a second server running. The extracted tree is gitignored. `browser-common.mjs` resets the synthetic seed DB for each phase, starts its standalone engine, owns the collector and bench URL, and reuses Gate 1 frame statistics. An occupied engine port 8080 is a blocker; never connect to someone else's engine. Concurrent operators coordinate whole runs with `mkdir /tmp/bistro-adr-8080.lock`, then `rmdir` even on failure; retry after 30 seconds if occupied.

### Gate 2.1 headed Playwright smoke

```bash
node docs/adr/data/0001/harness/run-playwright.mjs --browser chromium \
  --renderer docs/adr/data/0001/gate2/extracted-smoke/.vite/renderer/main_window \
  --seed /tmp/bistro-smoke-seed --out docs/adr/data/0001/gate2/playwright-smoke/chromium --runs 1
# Repeat with --browser webkit and a separate empty --out ending in /webkit.
```

Global Playwright **1.63.0** is resolved using `npm root -g` and imported by absolute path. Only its cached browsers are used: missing executables fail, no download occurs. Keep the headed browser visible/frontmost; headless rAF timing is meaningless. No host keystrokes are injected. Defaults are 30 startup runs and 10 frame runs per fixture; `--runs 1` is smoke only. Exact build IDs and floor analysis: `../gate2/tools.md`.

**Playwright WebKit is NOT system WKWebView.** This smoke catches renderer incompatibilities only; Gate 2.3's native host is the deciding renderer run. Each output has this warning, `run-manifest.json`, conditions, startup launch-to-collector receipt timings, frame summaries and per-page `diagnostics.json` (console, pageerrors, failed requests and HTTP errors). Timing starts before browser launch, after engine readiness: it is warm/no-purge and not comparable to Electron's hyperfine launch-to-exit. No memory values are invented. `completed.json` means all requested bench phases completed, not zero console diagnostics.

### Gate 2.2 API and CSS compatibility

```bash
sh docs/adr/data/0001/harness/compat.sh
node docs/adr/data/0001/gate2/compat/css-probe.mjs
```

`compat.sh` performs a frozen pnpm install with `--ignore-workspace` in the isolated `gate2/compat/` package, then lints `electron/src/renderer/**/*.{ts,tsx}` against Safari 16.1 (Ventura 13.0). Exact tool pins and lockfile are local, not Electron dependencies. Findings are evidence and do not fail collection; a broken sentinel, parse error or tooling error fails it. Outputs: `compat-results.txt`, `compat-results.json`, `compat-summary.md`; CSS output: `css-results.json`. CSS probes are functional/headless, not timing, and current Playwright support is not minimum-OS support. See `../gate2/tools.md` for primary sources and the CSS floor mismatch.

### Gate 2.3 native WKWebView host

```bash
swiftc docs/adr/data/0001/harness/webview-host/main.swift -o /tmp/bistro-webview-host
node docs/adr/data/0001/harness/run-webview.mjs --host /tmp/bistro-webview-host \
  --renderer docs/adr/data/0001/gate2/extracted-smoke/.vite/renderer/main_window \
  --seed /tmp/bistro-smoke-seed --out /tmp/bistro-webview-smoke --runs 1
node --test docs/adr/data/0001/harness/*.test.mjs
```

Host arguments are the bench URL and seed settings path; stdout is a JSONL protocol retained in `host.log`/`host-events.json`. The driver uses the current foreground/visibility protocol before measuring; focus or visibility loss invalidates the phase. Keep the host frontmost. `run-webview` owns engine/collector cleanup and records launch receipts, idle/post-diff memory and frame events; defaults are 30 startup, 5 memory, 10 frames per fixture. WebKit memory attribution depends on explicit process PID SPIs, not launchd ancestry. Missing PIDs or footprint failures must remain blockers, never partial memory totals. No app input scripts are involved.

### Gate 4 S6 keyboard matrix (passive until permission/layout preflight passes)

```bash
sh docs/adr/data/0001/harness/keys/build-native.sh
node docs/adr/data/0001/harness/run-keys.mjs --dry-run
node docs/adr/data/0001/harness/run-keys.mjs --preflight-only --limit 1 \
  --label passive --out /tmp/bistro-keys-preflight
node --test docs/adr/data/0001/harness/keys.test.mjs
```

These commands do not inject keys or change input sources. In development, the renderer URL can opt into `bench_keys=1` without a CLI flag; packaged measurement launches use the validated `--bench-query` path. Bench mode fails closed if its collector or synthetic fixture is invalid. Current permission/layout blockers are recorded in `../gate4/keys-preflight.txt`; **do not run a live matrix while blocked**. Preflight runs native `self-test`, `preflight`, `sources` and a passive System Events query. A successful Automation query is not permission to inject. Posting, Accessibility, passive tap and all required enabled sources are mandatory before target launch. Never edit the TCC database or silently enable layouts.

When separately authorized and preflight passes, a bounded synthetic smoke can use:

```bash
node docs/adr/data/0001/harness/run-keys.mjs --app /path/Bistro.app \
  --seed /tmp/bistro-smoke-seed --fixture-pr github:bench/large-diff:1 \
  --label baseline --out /tmp/bistro-keys-baseline --limit 4 \
  --style vim --layout US --actionId detail.approve
# Repeat against candidate in a separate output directory, then:
node docs/adr/data/0001/harness/keys-stats.mjs \
  --baseline /tmp/bistro-keys-baseline --candidate /tmp/bistro-keys-candidate \
  --out /tmp/bistro-keys-comparison.json
```

`--launch-command EXEC --launch-args '["arg"]'` launches any shell's executable without a shell interpreter (Wails must honor the same bench-query/user-data arguments); `--app` resolves `CFBundleExecutable`. `--pid PID` is parsed but refuses live attach until disposable-profile and queue ownership can be proven; use harness-owned launch instead. `--seed` copies settings/cache into the driver's output, disables sync/telemetry and sets rules-only local classification; SQLite is never edited; a read-only queue-count check rejects a copied seed with queued writes before launch. Renderer readiness must prove effective rules classification, dry dispatch, blocked external writes and the synthetic fixture. Renderer transport allows explicit view reads (some refresh only the disposable local cache) and the rules-only classification setter; provider sync, review/comment writes and other external actions are blocked. Live input guard loss aborts the entire run; the persistent native worker restores original input sources on stop, EOF or signals. No hotkey behavior is repaired for this measurement.

The shared main/renderer transport allowlist is audited against the engine protocol, not a `REQUEST_*` prefix:

- `AUTH_CHECK`: renderer bootstrap authentication status.
- `REQUEST_PR_LIST`, `REQUEST_REPO_LIST`, `REQUEST_REVIEW_QUEUE`, `REQUEST_TRIAGE`: populate local review/list views; provider reads and disposable-cache refreshes are allowed.
- `REQUEST_DIFF`, `GET_PATCHSETS`, `REQUEST_INTERDIFF`, `CHECK_PR_CHECKOUT`: display diff/patchset and local checkout state.
- `REQUEST_COMMENTS`, `GET_REVIEW_THREADS`, `DRAFTS_LIST`: display cached discussion/drafts; comment classification kicks exit in confirmed rules mode.
- `GET_EXPERTISE`, `GET_EXPERTISE_SETTINGS`: stored expertise/settings reads, not model execution.
- `SET_CLASSIFICATION_SETTINGS`: the only permitted setter, strictly `mode: rules` and `dismissal_scope: local`; it prevents external classification/dismissal behavior from first main-process connection and on reconnect.

All other message types are refused before either socket sends, including PR sync, review/comment/draft writes, queue submissions, and unknown future messages. Renderer `send` stays fire-and-forget: refusal is visible in its log and the `keys_blocked_write` collector event, not a return value. Requests return a tagged fault. External/editor IPC, native quit, palette-only commands and action handlers are guarded separately; ordinary launches are unchanged. The normal renderer HTML still makes anonymous Google Fonts GET requests; these are known outbound reads, not writes.

Source inventory reuses Gate 3's text parser, excludes platform-disabled `global.quit.win` and adds textarea-local `Mod+Enter` plus the independently accepted macOS `Control+Enter` chord in all styles. Scope is US layout only, with no IME dimension (owner amendment, 2026-10-28). Full universe: **1,448 cells per shell** (vim 488; emacs 496; standard 464). `--dry-run` prints counts and ten selected cells by default. Live runs require explicit `--limit 1..10`; only `--full` permits a larger filtered/full matrix. Quit/Cmd+Q cells remain in the denominator but are never injected: manual check pending. The 250 ms per-step pacing gives a hypothetical full-universe lower bound **434 seconds (~7.2 minutes) per shell**, excluding setup/collector/native latency, not a measured runtime.

`run-manifest.json` fingerprints target/options/chords/cell dimensions; `--resume` rejects a changed identity and skips only complete groups. `cells.jsonl` groups native tap, DOM events, intended actionIds and resulting text per exact cell; raw `renderer.jsonl`, `native-tap.jsonl` and `worker.jsonl` remain beside it. Stats applies the frozen ADR overlapping class tolerances to the measured classes, flags core review regressions, and compares Gate 3 predictions. Full-source coverage is always required for a gate PASS: smoke, missing observations, unreachable handlers, fake textareas and failed setup remain **incomplete**, never equal no-op passes. `completed.json` means the requested driver loop ended, not S6 passed.

The non-US Option and IME-active classes are out of scope and not measured, so they are neither passed nor failed. The Gate 3 IME finding (`isTextareaSubmit` has no `isComposing` guard) remains a Bistro bug in both shells. Renderer fixture does not reach every overlay/onboarding/login handler, so those cells explicitly remain incomplete. No full S6 verdict is claimed.
