# ADR 0001 results: Gates 1–4

Results for [ADR 0001](0001-electron-vs-wails.md), judged against its pre-registered thresholds.

Gate 0 originally stopped here. After the override (see Amendments), Gates 1–4 ran on this machine (warm starts, 60 Hz display, AC power; `conditions.txt` in each run). Full runs use the pre-registered counts and one seed (`/tmp/bistro-bench-seed`: both pinned real PRs and both synthetic shapes). The S1/S3/S4/S5 numbers below come from `node docs/adr/data/0001/harness/evaluate-shell.mjs <slim> <candidate> [app KiB] [DMG bytes]`, with output in `data/0001/gate2/shell-thresholds.json` and `data/0001/gate4/shell-thresholds.json`.

Baseline: `data/0001/gate1/slim` (S thresholds compare against slim); `data/0001/gate1/current` is recorded alongside it. Candidates: the native WKWebView host (`data/0001/gate2/wkwebview-full`) and the Wails v3 spike (`data/0001/gate4/wails-full`), both running the exact slim renderer bundle.

**S1, frames (current macOS only).** Pooled p95 / p99 in ms, late-frame share, and median per-run p95:

| Fixture | Slim (limit: p95 +2, p99 max(×1.1, +3), share +2 pp, run p95 +2) | WKWebView | Wails |
|---|---|---|---|
| kubernetes:143358 | 27.9 / 34.9, 5.55%, 27.9 | 29 / 36, 5.89%, 29 — pass | 27 / 32, 5.40%, 27 — pass |
| google-cloud-go:20526 | 18.7 / 37.9, 1.75%, 18.6 | 22 / 38, 2.51%, 21.5 — **fail** (p95, run p95) | 19 / 35, 1.75%, 19 — pass |
| synthetic 1,000 files | 35.5 / 38.3, 10.56%, 35.5 | 37 / 38, 10.64%, 37 — pass | 35 / 37, 10.61%, 35 — pass |
| synthetic one file | 18.5 / 18.7, 0.01%, 18.1 | 18 / 19, 0.04%, 18 — pass | 19 / 19, 0.00%, 19 — pass |

WebKit reports rAF timestamps in whole milliseconds; Chromium reports 0.1 ms. That can move a WebKit quantile by up to 1 ms; the WKWebView google-cloud-go p95 misses its limit (20.7) by 1.3 ms. S1 also requires macOS 13, which the owner declined, so S1 is **incomplete** for both candidates even where the current-macOS rows pass.

| Threshold | Slim | WKWebView | Wails | Result (Wails) |
|---|---|---|---|---|
| S2 startup, p50 / p95 (ms) | 1,628 / 1,767 (launch to PR list, then exit) | 1,236 / 1,395 | 1,314 / 1,403 | **Not comparable.** The WebKit drivers time spawn to PR-list receipt with the engine already running; there is no exit-timed equivalent. |
| S3 idle memory, median / max (MiB), limit median ≤ 173.3 and max < slim min 247.2 | 247.6 median, 247.2 min | 150.1 / 151.0 | 149.1 / 152.0 | pass |
| S4 memory after google-cloud-go (largest real fixture), median (MiB), limit ≤ 603.4 | 754.3 | 520.0 | 544.9 | pass |
| S5 size, `.app` KiB / DMG bytes, limit ≤ 118,488 / ≤ 40,520,512 | 296,220 / 101,301,280 | not packaged | 70,624 / 28,023,742 | pass, but for an unsigned spike with no updater, icons or notarization |
| S6 keybindings (US only, owner amendment) | — | — | — | **Not run.** Preflight blocks on Accessibility/event posting and System Events permissions (`data/0001/gate4/keys-preflight.txt`). |
| S7 upstream health | — | — | — | **fail** (Gate 3) |
| S8 continuity | — | — | — | **Not evaluated** (Gate 5 skipped: no Developer ID) |

Electron `current` vs `slim`: startup p50 1,571 vs 1,628 ms; idle memory 247.7 vs 247.6 MiB; `.app` 297,168 vs 296,220 KiB; DMG 102,227,881 vs 101,301,280 bytes; frame quantiles within 0.3 ms. Slim buys almost nothing.

Reading: on this machine, Wails matches Electron's frame times and uses about 100 MiB less at idle and about 210 MiB less after the largest diff, with a DMG under 30% of slim's size. That is the whole benefit. It doesn't reverse S7, and S1 (macOS 13), S2, S6 and S8 stay open, so the shell verdict stays no-go.

Harness failures during the full runs, both fixed and kept out of the evidence: an inherited `ELECTRON_RUN_AS_NODE=1` made the unfused `current` build start as Node (`28dd179`); process-group cleanup raced zombie reaping and aborted two Gate 1 runs with EPERM (`4aeb381`).

Still to do:
- **S6 live keyboard matrix:** needs the owner to grant Accessibility and System Events to the agent's host process.
- **No second machine:** none is available, and macOS 13 was declined.
- **True cold starts:** they need `sudo purge` without a password; all startup runs are warm and labelled as such.
- **The blind alpha-tester session.**
