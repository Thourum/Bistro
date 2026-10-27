# ADR 0001 Gate 1 harness plan

Goal: measure the current and slim Electron builds (size, startup, memory, large-diff frame times) with one harness. The same harness is reused in Gates 2 and 4 for Playwright, the WKWebView host and Wails. ADR: `docs/adr/0001-electron-vs-wails.md`.

## Shared protocol (all tasks code against this)

The renderer reads **URL query parameters** (`location.search`, before the hash). Without them the bench code is inert, and it is never loaded: it's a dynamic `import()`.

| Param | Meaning |
|---|---|
| `bench_collector=http://127.0.0.1:PORT` | Where to POST results. Required to activate the bench. |
| `bench_quit_after_ready=1` | After `pr_list_rendered`, call `window.electron.quitApp()` if it exists. Other hosts are killed by the driver. |
| `bench_diff=<prId>` | After the list renders, open `#/prs/<encodeURIComponent(prId)>/diff` and run the frame harness. |
| `bench_frames=<N>` | Max scroll frames per run (default 1200). |
| `bench_step_px=<px>` | Scroll distance per frame (default 48). |

When the bench is active and the URL has no hash, the renderer starts at the PR list with the filter that shows all PRs. The seeded PRs are merged, and the default filter would hide them.

Every event is `POST {collector}/event` with a JSON body, `Content-Type: text/plain`. That avoids a CORS preflight; the collector replies with `Access-Control-Allow-Origin: *`.

- `{event:"pr_list_rendered", t, timeOrigin, rows}` is posted once, the first time the PR list has rendered at least 1 row (`t` = `performance.now()`).
- `{event:"frames", prId, refreshMs, deltas:number[], scrolledPx, scrollHeight, viewportHeight, userAgent}`, collected as follows:
  1. Wait for `[data-diff-viewport]` with `scrollHeight > clientHeight`, and wait until its `scrollHeight` has been unchanged for 1000 ms (cap the wait at 30 s).
  2. Record 120 idle rAF deltas. Their median is `refreshMs`.
  3. Scroll by setting `scrollTop += step` once per rAF, stopping at the bottom or after N frames. Record each rAF delta (`performance.now()` differences).
- `{event:"diff_done"}` is posted after `frames`. The page then stays idle so the driver can sample memory.
- `{event:"error", message}` is posted on any harness failure (for example, the viewport never appears).

Electron main argv:
- `--quit-after-ready` adds `bench_quit_after_ready=1`.
- `--bench-query=<urlencoded query string>` appends the decoded string to the window URL's search. Use `loadFile(path, { search })` when packaged; when using `loadURL`, append it to the URL.
- Engine data: the packaged app derives the DB from `app.getPath('userData')`. The driver passes `--user-data-dir=<dir>`. Verify that Electron 44 honours it for `userData`. If it doesn't, add the smallest equivalent override.

Pinned fixtures: both are merged, so `refs/pull/N/head` is fixed. The seeding step asserts the head SHAs.

| Fixture | PR id | head SHA |
|---|---|---|
| vendoring | `github:kubernetes/kubernetes:143358` | prefix `09048f5ccf2c` |
| generated | `github:googleapis/google-cloud-go:20526` | prefix `de1e8de4f65e` |
| synthetic | `github:bench/large-diff:1` (1000 files × 10 changed lines) and `github:bench/large-diff:2` (1 file × 10000 changed lines), the `bench:large-diff` scenarios | written by `go/cmd/bistro-benchseed` through the store writer, so Go still owns writes |

## Tasks

1. **Renderer + main bench hooks** (`electron/src/main/*`, `electron/src/renderer/bench/*`, a one-line hook in `PRListPage.tsx`). Put the pure stats (percentiles, dropped-frame share) in a pure module with one focused test.
2. **Driver scripts** (`docs/adr/data/0001/harness/`):
   - **Seed:** a standalone engine on a temp DB and repo dir, with `-auto-sync=false`. It sends `REQUEST_PR_SYNC` for the pinned PRs and asserts the head SHAs and diff row counts. The result is a seed dir that is copied fresh for every run.
   - **Collector.**
   - **Startup:** `hyperfine` to exit under `--quit-after-ready`. It uses `--prepare 'sudo -n purge'` when available; otherwise warm runs, labelled as such.
   - **Memory:** `footprint` summed over the whole process tree, including `bistro-sync`, both idle and after the large-diff run.
   - **Frame runs:** n runs per fixture.
   - **Sizes:** DMG and installed `.app`.
   - Raw output goes to `docs/adr/data/0001/gate1/<build>/`.
3. **Slim build** (`electron/forge.config.ts`, `electron/package.json`):
   - When `BISTRO_SLIM=1` is set: Electron fuses via `@electron-forge/plugin-fuses`, delete the `.woff` duplicates of the `.woff2` fonts from the packaged renderer, and keep the existing ASAR and locale stripping.
   - Remove the unused `ts-deepmerge` dependency (knip).
4. **Synthetic seeder** (`go/cmd/bistro-benchseed`): `bistro-benchseed -db-path X` runs migrations, then writes the two synthetic PRs (open state, title `bench: …`) through the same writer sync uses. The generated diff text matches `electron/scripts/bench-large-diff.mjs`.

Verification: `pnpm type-check && pnpm lint && pnpm test` in `electron/`; package both builds; one smoke run of each driver step.
