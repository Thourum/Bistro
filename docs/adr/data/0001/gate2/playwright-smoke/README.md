# Gate 2.1 n=1 headed smoke

**Playwright WebKit is NOT system WKWebView.** This is a compatibility smoke test; Gate 2.3's native host is the deciding renderer run. Synthetic-only seed, n=1 per phase, external 60 Hz display, macOS 27.2. No minimum-OS or shell verdict. Exact tools: ../tools.md.

| Engine / synthetic fixture | Frames | p50 ms | p95 ms | p99 ms | Late share (measured refresh × 1.5) |
| --- | ---: | ---: | ---: | ---: | ---: |
| Chromium / 1000 files | 1200 | 16.60 | 28.82 | 31.80 | 9.83% |
| Chromium / one file | 1200 | 16.70 | 17.20 | 17.60 | 0.00% |
| Playwright WebKit / 1000 files | 1200 | 17.00 | 38.00 | 43.00 | 10.75% |
| Playwright WebKit / one file | 1200 | 17.00 | 19.00 | 21.00 | 0.33% |

Startup browser-launch-to-`pr_list_rendered` collector receipt: Chromium **4078.63 ms**, WebKit **4695.39 ms**, one launch each. Engines were ready before browser launch; warm/no purge. Not comparable to Electron hyperfine exit timings.

Console errors: **0** both engines. Pageerrors / HTTP errors: **0** both. Chromium records **7 failed collector requests**, all `net::ERR_ABORTED` (one startup, three each diff phase); retained rather than hidden. All target the temporary collector, consistent with teardown cancellation; diagnostics do not timestamp them, so teardown attribution is not proven. No renderer asset failure was recorded. WebKit records **0** failed requests. Info console messages: Chromium 28, WebKit 34. Raw events, launch receipts and all diagnostics are retained under each engine directory.

WebKit first attempt could not launch because the expected cached revision 2359 was missing. The coordinator installed that build before this successful smoke; this driver downloaded nothing. Both completed.json markers exist. The two pinned real PR fixtures remain unseeded (engine keychain returns 401); they require owner reauthentication and later full-count runs.
