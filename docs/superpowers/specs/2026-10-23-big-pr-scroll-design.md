# Big PRs That Open and Scroll Smoothly — Design

**Date:** 2026-10-23
**Status:** Approved (user-reviewed design conversation)
**Roadmap:** Review-features priority #1 (slow UI: 31 tags in `docs/marketing/04-context-and-evidence.md` §4.3)

## Summary

Bistro must open and scroll 1,000+ file PRs without stalls. A measurement
spike on MarlinFirmware/Marlin#28644 (2,598 files, +271k/−156k, ~19 MB unified
diff) showed that **load is fine (<1 s)** but fast scrolling locks the UI for
1–3 s after ~5 s. The causes are known and each one has a targeted fix. Also,
files past the first 4 MB of a PR render blank, which this design removes.

## Measurements (spike, 2026-10-22/23)

Engine side (go-git, merge-base diff): diff generation and JSON encode are not
the bottleneck. Renderer open pipeline (`JSON.parse` → zod → `splitUnifiedDiff`
→ `parseCodeViewFiles` → `buildCodeViewItems`) completes well under 1 s.

Scroll trace (Chrome performance trace, dev build): 10 long tasks (>50 ms),
3.2 s of main-thread time total, worst single task 693 ms.

| Share | Cause | Fix |
|---|---|---|
| ~32% | Shiki syntax highlighting + word-diff in @pierre/diffs run on the **main thread** (`disableWorkerPool` in `components/diff/DiffView.tsx`) | Enable the worker pool (§3) |
| 26% | React dev-mode overhead (`console.createTask`, `jsxDEV`) | Disappears in production; acceptance is measured there |
| 25% | React reconcile/commit: `components/diff/diffFileSidebar.tsx` renders **all 2,598 rows**, unvirtualized and unmemoized, on every active-file change | Virtualize + memo (§2) |
| 11% | `DiffPageStatusBar` runs `diff.split('\n').length` over the 19 MB string **on every render** (`pages/DiffPageChrome.tsx`) | Memoize (§2) |

`findTopVisibleFile` (scroll handler) measured 0 ms — not a cause.

The "fine for 5 s, then locks" pattern: every active-file change re-renders the
full sidebar and status bar while newly visible files queue main-thread
highlighting. Fast scrolling through many small files stacks both.

## Design

### 1. Remove the per-PR byte cap (Go)

- Drop `DefaultMaxTotalPatchBytes` (4 MB, `go/internal/git/types.go`) from
  `DefaultDiffOptions()` and the `MaxTotalPatchBytes` budget branch in
  `renderDiffFile` (`go/internal/git/diff.go`). Delete the option entirely if
  nothing else sets it.
- Keep the 512 KB per-file cap (`DefaultMaxFilePatchBytes`). Files over it keep
  `diff_status = 'too_large'`.
- Renderer: a `too_large` file renders a placeholder row instead of a blank
  body: `File too large (N KB) — o open on GitHub`, using the existing `o`
  binding.

### 2. Stop whole-tree re-renders (renderer)

- **Status bar:** compute the line count once per diff with `useMemo` (or sum
  per-file additions + deletions, which are already in `DiffFile`). Never split
  the full diff string during render.
- **File sidebar:** virtualize with `@tanstack/react-virtual` (already a
  dependency) and `React.memo` each row, so an active-file change re-renders
  two rows, not 2,598. Keyboard `Shift+J`/`Shift+K` file navigation must
  scroll the virtual list to the active row.

### 3. Highlight off the main thread (renderer)

- Remove `disableWorkerPool` and provide @pierre/diffs' worker pool
  (`WorkerPoolContextProvider` + `workerFactory`) with a Vite `?worker` import
  that works under Electron Forge's Vite build (dev and packaged).
- Set `tokenizeMaxLineLength` so minified/generated lines don't stall a worker.
- Update `docs/superpowers/plans/2026-07-18-codeview-spike-notes.md`, which
  recorded the worker pool as a deliberate deferral.

### 4. Regression guard

- Go: one test generating a ≥1,000-file synthetic diff with default options,
  asserting every file is present and none is `too_large` solely for budget
  reasons (the bug this fixes).
- Renderer: one debug log line on diff open with file count, payload bytes,
  and time-to-first-file, so regressions show in user logs.

### 5. Acceptance

In a **production build** (`pnpm run package`), opening Marlin #28644:

- first file visible within **1 s** of navigation;
- a fast top-to-bottom scroll produces **no main-thread task over 100 ms**
  (Chrome performance trace).

## Out of scope

- Merge-base diffing — covered by
  `2026-10-23-since-last-review-design.md` §1.
- Cold-clone sync time for huge repos.
- Server-side pagination/streaming of `DIFF_READY` — not needed per the
  measurements; revisit only if a PR fails the acceptance numbers after these
  fixes.

## Testing

Per project convention (no React component tests, no tests that exist only to
exist): the Go regression test in §4. Sidebar virtualization and worker pool
are verified manually against the acceptance criteria.
