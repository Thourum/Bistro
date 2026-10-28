# Gate 3 — S7 upstream health

Part of the Gate 3 read; see [README.md](README.md) for pins and the S6 prediction.

## S7 — upstream health

Interval `[2025-10-27, 2026-10-27)`. Data: [health-summary.json](health-summary.json), produced by [health.sh](health.sh) / [health.mjs](health.mjs).

| Criterion | Threshold | Figure | Result |
|---|---|---|---|
| Maturity + stability statement | beta or better, stated | `v3.0.0` stable; README stability statement | **PASS** |
| Releases in 12 months | ≥ 6 | 129 non-draft releases, 121 of them `v3.*` | **PASS** |
| Top committer share of non-bot commits | ≤ 70% | leaanthony 492 / 742 = **66.31%** (automation names excluded); 54.04% of 929 when only API-flagged bots are excluded | **PASS** (narrow) |
| Median first maintainer response, last 100 issues | ≤ 14 days | 47 / 100 answered. Respondent-only median 15.30 d; lower bound (unanswered = age at cutoff) 18.89 d; conservative median not identifiable (Infinity) | **FAIL** |
| No open macOS keyboard issue in an S6 class without a workaround | none | #6033 (Fn/Globe swallowed before IME) is open, with no confirmed workaround | **Possible FAIL** — see below |

Method notes:

- Commits are attributed to the author login. Bots are API `Bot` accounts, `[bot]` / dependabot / github-actions identities, and six exact automation author names listed in the summary. Ten commits use the author *name* `taliesin-ai` under the leaanthony login, and the name match excludes them. Counting them as human raises the share by about half a point; the result is still a PASS.
- A maintainer response is the first OWNER / MEMBER / COLLABORATOR comment by someone other than the reporter. Closures and reactions do not count. Many of the counted responses come from accounts that look automated.
- Sensitivity check, not saved: excluding Bot users and `taliesin-ai` from responders leaves 5 of 100 issues with a human maintainer reply, all from leaanthony. Their median is 12.50 d, but the lower bound over all 100 is 26.31 d.
- **S7 fails under every response-time definition tried.**
- #6033 is Fn/Globe, which is not a Bistro binding. It touches the IME class only through dictation and the Globe input-source switch. The issue does not state a version, and its proposed patch is unverified. Whether it counts against S7 is a judgement for the ADR author; it is disclosed rather than decided here.

### Keyboard / WebKit / WebKitGTK issues (snapshots: `issue-N.json`, `issue-N-comments.json`)

| # | State (dates) | Version | Class | Workaround / fix |
|---|---|---|---|---|
| [6033](https://github.com/wailsapp/wails/issues/6033) | open (2026-08-26) | unspecified | macOS Fn/Globe → IME | none confirmed |
| [6056](https://github.com/wailsapp/wails/issues/6056) | open (2026-08-30) | v3 beta.1 | macOS 60 FPS cap on ProMotion | `_features` preference, confirmed by reporter 2026-09-02 |
| [6170](https://github.com/wailsapp/wails/issues/6170) | closed 2026-10-15 | v3 beta.25 | macOS 27 `WindowDidUpdate` flood | stop posting the event |
| [6121](https://github.com/wailsapp/wails/issues/6121) | open (2026-09-14) | v3 beta.19+ | macOS inspector regression (PR #6087) | Safari remote inspector |
| [6249](https://github.com/wailsapp/wails/issues/6249) | open (2026-10-11) | v2.16.0 | macOS keyboard focus order (a11y) | none; v2 |
| [4918](https://github.com/wailsapp/wails/issues/4918) | closed 2026-02-24 | v2.11.0 | custom menu kills clipboard shortcuts in inputs | add Edit role menu |
| [4061](https://github.com/wailsapp/wails/issues/4061) | closed 2025-04-19 | v3 alpha.9 | cannot type in inputs (Monterey) | CSS `user-select` cause |
| [3722](https://github.com/wailsapp/wails/issues/3722) | closed 2024-09-12 | v3 alpha.4 | Cmd+A closed dialogs | fixed in v3-alpha |
| [2604](https://github.com/wailsapp/wails/issues/2604) | closed 2023-05-10 | v2.4.1 | macOS keystroke frequency | v2, historical |
| [5527](https://github.com/wailsapp/wails/issues/5527) | closed 2026-06-03 | v3 alpha.97 | Linux inspector freeze | fixed |
| [5631](https://github.com/wailsapp/wails/issues/5631) | closed 2026-07-01 | v3 alpha2.103 | WebKitGTK/JSC SIGABRT/SIGSEGV (GTK thread safety) | PRs 5640 / 5668 |
| [4957](https://github.com/wailsapp/wails/issues/4957) / [5459](https://github.com/wailsapp/wails/issues/5459) | closed 2026-04-29 / 2026-05-16 | v3 | GTK4 + WebKitGTK 6.0 becomes the default for GA | GTK3 opt-in |
| [4985](https://github.com/wailsapp/wails/issues/4985) | closed 2026-05-02 | v3 | blank window, X11 + NVIDIA | env-var workaround |
| [5522](https://github.com/wailsapp/wails/issues/5522) | closed 2026-05-31 | v3 GTK4 | WebKitGTK 2.52 SoupMessage use-after-free | PR #5521 |
| [6219](https://github.com/wailsapp/wails/issues/6219) | open (2026-10-04) | v3 beta.27 | first paint delay | Windows-only, out of scope |
| [4794](https://github.com/wailsapp/wails/issues/4794) | closed 2026-01-07 | v3 | runtime version mismatch (422) | not keyboard, out of scope |

