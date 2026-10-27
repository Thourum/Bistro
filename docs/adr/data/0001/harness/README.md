# Gate 1 measurement drivers

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
