# First run, spec 1 of 4: loading and sign-in — design

Status: approved in brainstorming 2026-10-24.

## The first-run program

The product and design handoff redesigns the whole first run. It is split into
four specs, built in flow order:

1. **Loading and sign-in** (this spec)
2. **Step 1/2, choose what to sync**: local clones first, GitHub repos
   lazy-loaded, needs-you counts shown only when something needs you. Continue
   saves the selection and starts the sync.
3. **Step 2/2, style + practice**: vertical picker, 3 stages / 17 tasks, a live
   sync footer. It runs while the sync from step 1 works.
4. **First-run rail**: a non-modal 3-step rail over the live inbox. The
   5-screen modal stays as `:tour`.

The design reference is the handoff bundle, kept out of git at
`design/onboarding-flow/` (`Onboarding Flow.dc.html`, frames 1a–1j for this
spec). The design is a layout and look-and-feel reference. Its action names,
key choices and some copy are placeholders. Where it disagrees with this spec,
this spec wins.

## Goal

The first two screens should explain themselves and never leave the user stuck:

- a slow or crashed engine says why and offers a way out;
- sign-in says exactly what GitHub will ask for and where the token goes;
- a pending sign-in can be cancelled, reopened or copied, and it times out
  visibly;
- the browser tab the user lands on after GitHub is branded and useful.

## Loading `[0]` (frames 1a–1c)

- **Initialising and connecting (1a, 1b):** unchanged.
- **Slow state (1c):** shown after `SLOW_ENGINE_MS = 10_000` without a ready
  engine. A healthy start, including migrations, usually finishes in under a
  second, so 10 s means something is unusual without firing on a normal cold
  start. The timer counts from the first spawn and does not reset when the
  engine is automatically restarted. The screen shows:
  - an elapsed timer (`CONNECTING… 00:14`);
  - one line explaining the wait ("taking longer than usual … after an update
    or on first launch");
  - the last 3 lines of the engine log;
  - **Open log folder** (`L`).

  **Restart engine** (`R`) appears only once the log tail has not changed for
  `STALLED_ENGINE_MS = 20_000`. A tail that is still moving means migrations
  or indexing are running, and restarting would interrupt them. A tail that
  has gone quiet means the engine is stuck, for example waiting on the lock.
- **Crashed state (new, not in the design):** the engine has exited and the
  automatic restarts (`MAX_RESTARTS = 3` in `go-spawn.ts`) are used up. This
  state belongs to the engine, not to a route: it replaces whatever screen is
  showing (loading, sign-in, or later screens) until the engine is ready again.
  It shows:
  - "The sync engine stopped";
  - the exit code or signal, and the log tail;
  - **Restart engine** (`R`), **Open log folder** (`L`) and **Report the
    crash** (`I`).
- **Log tail source:** the engine log file, falling back to the engine's
  stderr lines that main has captured (`go-spawn.ts` pipes stderr). That
  covers a binary that dies before it opens its log, for example a missing
  library.
- **Restart with an external engine:** when Electron connected to an engine
  it did not spawn, it can't restart it. **Restart engine** is hidden, and the
  copy says to restart the running `bistro-sync`.
- **Report the crash** is offered only in the crashed state. Slow and
  engine-down states are still recovering on their own, so a report there
  would usually describe a transient problem.

  The report holds:
  - app version, OS and architecture, and the engine exit code or signal;
  - the last `REPORT_LOG_LINES = 60` log lines, in a fenced block. 60 lines
    covers start-up plus the failure for a typical crash, and fits the URL
    limit.

  Redaction runs before anything leaves main:
  - GitHub token patterns (`gh[pousr]_…`, `github_pat_…`), `Authorization:`
    header values and `code=`/`state=` query values become `[redacted]`;
  - the home directory prefix becomes `~`.

  Repo names, PR titles and usernames in the log are not redacted. The button
  says so before acting: "Opens a GitHub issue with the last 60 log lines.
  Check them for private repo names before you submit."

  Where the report goes depends on the `IS_SOURCE_PUBLIC` constant, the same
  one that gates the "read the source" link:
  - **Public repo:** open
    `https://github.com/Itterno/Bistro/issues/new?title=…&body=…` (reuse the
    `BISTRO_UPDATE_REPO` constant). The oldest lines are trimmed until the URL
    is under `MAX_ISSUE_URL_BYTES = 7_500`. GitHub rejects new-issue URLs much
    past 8 KB, so this leaves margin. GitHub's new-issue URL can't attach
    files. The user sees and edits the issue before submitting.
  - **Private repo (today):** a URL to a private repo gives outside users a
    404. Instead, copy the same redacted report to the clipboard and say
    "Crash report copied. Send it to the Bistro team." The label reads **Copy
    crash report**.
- **Main-process IPC (new):**

  | Channel | Does |
  |---|---|
  | `engine:restart` | resets the restart counter and respawns through `go-spawn`'s existing path |
  | `engine:log-tail` `{ lines }` | returns the last N lines of `<dataDir>/logs/bistro-sync.log` |
  | `engine:open-log-folder` | `shell.openPath` on the log directory |
  | `engine:report-crash` | builds the redacted report in main (main owns the log); opens the issue URL or copies the report, per `IS_SOURCE_PUBLIC` |

  Each handler returns a typed result, and the renderer maps failures to a
  toast. Main already sends `go:crashed`, but `preload.ts` doesn't allow the
  channel. Allow it, and add an `engine:status` push (`starting | slow |
  ready | crashed`, with exit info) so the loading screen doesn't poll.

## Sign-in `[1]` (frames 1d–1h)

### Layout

A two-column card.

- **Left:** the logo, "Connect your platform to begin reviewing code", the
  host list, the state area, and the footnote "Sign-in opens GitHub in your
  browser. Bistro never sees your password."
- **Right, in every state:** the trust panel.

Below `md` the card stacks to one column, with the trust panel second.

### Hosts

- GitHub is focused and gets `G`.
- GitLab and Gitea are disabled. They read "Coming soon", have no key hint,
  and ignore clicks.
- `PROVIDERS` keeps all three, with an `isAvailable` flag. Only `github` is
  true.

### Trust panel

- **What GitHub will ask you to approve:** `repo` ("GitHub labels this 'full
  control of private repositories'. Bistro needs it to read the private PRs
  you review and to post your reviews.") and `read:user` ("Your username, so
  Bistro knows which review requests are yours."). These match
  `handler/auth.go`.
- **Where the token goes:** three steps.
  - Step 2 depends on the exchange mode. In proxy mode (no
    `GITHUB_CLIENT_SECRET`, as in packaged builds) it reads: "Bistro's sign-in
    endpoint swaps it for a token and passes it straight back. It stores
    nothing." In local mode it reads: "Bistro swaps it for a token on this
    machine."
  - The engine reports the mode as `exchange: 'proxy' | 'local'` in the
    `AUTH_CHECK` reply. Until that reply arrives, and while the engine is
    down, step 2 uses mode-neutral copy: "Bistro swaps it for a token." It
    never guesses a mode.
  - Step 3 names the OS store: macOS Keychain, Windows Credential Manager, or
    the Secret Service keyring on Linux.
  - Never claim the token "never leaves this machine".
- **Facts:**
  - Telemetry: off by default (`privacy.telemetryEnabled: false`).
  - Auto-update: off by default (requires `BISTRO_ENABLE_AUTO_UPDATES=1`).
  - Licence: GPLv3, with a "read the source" link to the repo. Hide the link
    behind `IS_SOURCE_PUBLIC` (false today: the repo is private).
  - Revoke: GitHub → Settings → Applications.

### States

| Frame | State | Left column shows | Keys |
|---|---|---|---|
| 1d | idle | hosts | `G` |
| 1f | waiting | "Waiting for authorization… GITHUB // 00:23", "Complete the sign-in in your browser, then return here." | `R` Reopen browser, `C` Copy sign-in link, `Esc` Cancel |
| 1g | timed out | "No response after 5:00", "The browser hasn't reported back, and Bistro stopped waiting. Start again to get a fresh sign-in." | `↩` or `G` Try again, `Esc` Back to hosts |
| — | failed | hosts plus an inline error with the engine's message | `G` |
| — | interrupted | hosts plus "Sign-in was interrupted because the sync engine restarted. Start again." | `G` |
| 1h | engine down | hosts plus an inline alert ("Cannot connect to sync engine. Bistro is restarting it.") and a status block: engine state, error detail, last sync, version | `G` (disabled while down) |

The engine-down state covers only the time while automatic restarts remain.
Once they are used up, the crashed state replaces the sign-in screen (see
Loading). Its alert therefore never promises a restart that won't happen.

Supporting changes:

- **Waiting:**
  - The renderer keeps the `AUTH_REDIRECT` URL in `appStore` instead of
    passing it to `openExternal` and dropping it.
  - Reopen calls `openExternal(url)` again. Copy writes the URL to the
    clipboard.
  - Elapsed time counts from `AUTH_REDIRECT`.
- **Cancel:** a new `AUTH_CANCEL` WS message.
  - The engine closes the pending callback server and clears the
    in-progress flag, so the next `AUTH_START` works instead of returning
    `auth_already_in_progress`.
  - It broadcasts `AUTH_FAILED { error: 'cancelled' }`, and the renderer
    returns to idle without showing an error.
- **Timeout:** the engine already fails after `auth.CallbackTimeout = 5m` with
  `AUTH_FAILED "timeout"`. The renderer maps that error to the timed-out state.
- **Failed:** other `AUTH_FAILED` errors go back to idle with an inline error.
- **Interrupted:** the timeout and the callback server both live in the
  engine. If the WS disconnects or the engine restarts while sign-in is
  waiting, nothing would ever end the wait. So any WS disconnect during
  waiting moves the auth slice to interrupted, and drops the stored sign-in
  URL.
- **Late callbacks:** after a cancel or a timeout, the callback server is gone.
  A user who approves on GitHub afterwards gets the browser's own connection
  error, not the branded page. This is accepted; the app already shows the
  timed-out or idle state with a way to start again.
- **Engine down:**
  - The status block shows only while the WS is disconnected. Otherwise the
    status stays in the bottom bar.
  - The version comes from the app version (`app.getVersion()` via the
    existing settings/IPC path), not the hard-coded `v0.1.0-stable`.

The auth screen state is a pure function of the auth slice plus the
connection status:

```
(auth slice, ws connected) -> 'idle' | 'waiting' | 'timedOut' | 'failed' | 'interrupted' | 'engineDown'
```

It lives in `utils/` and is unit-tested.

## Browser callback `[1b]` (frames 1i–1j)

`go/internal/auth/callback.go` serves branded, self-contained HTML:

- one embedded template;
- inline CSS using the design tokens' light palette as literal values (this
  page is outside the app's token system);
- the system font stack;
- no icon font and no external requests.

### Success (1i)

- `AUTHORIZATION COMPLETE // GITHUB`, followed by "You're signed in."
- "Close this tab and return to Bistro."
- A **Where the token went** block with the same three steps, in the past
  tense, OS- and mode-aware.
- "Revoke any time: github.com/settings/applications."

Don't promise "already started reading your review requests". Spec 2 owns
when sync starts.

### Failure (1j)

- A red code line, then "Sign-in didn't finish."
- One plain sentence for the case, then "Nothing was saved. Return to Bistro
  and press `G` to try again."
- A **Detail** block with `error`, `description` and `at` (RFC 3339 UTC).

Cases:

| Case | Code | HTTP |
|---|---|---|
| state mismatch | `INVALID_STATE` | 403 |
| GitHub returned `?error=` (for example `access_denied`) | GitHub's code | 400 |
| missing code | `MISSING_CODE` | 400 |
| exchange failed | `TOKEN_EXCHANGE_FAILED`, with the upstream `error` / `error_description` when present | 500 |

All values are HTML-escaped. The engine still emits `AUTH_FAILED` for each case.

## Keys

The new screen actions join the action catalog (`utils/keybindings.ts`) on
routes `login` and `connecting`:

- loading: `R` restart, `L` log folder, `I` report;
- sign-in: `R` reopen, `C` copy, `Esc` cancel or back, `↩` try again.

They go in `SHARED_KEYS`, identical in every style, because the style is picked
later. Activation is limited to the matching screen state through
`isBindingEnabled`, as the onboarding bindings are. The connecting screen needs
a `RouteBase` (`'connecting'`). Add it where `routeBaseFromPath` maps the
pre-auth screens.

These keys must work before the WebSocket is up. Settings, including key
overrides and the navigation style, load from main over IPC (`initSettings`),
not from the engine. So `useAppHotkeys` has to be mounted on the loading and
crashed screens and must not wait on the engine. The plan verifies that; if
the hook is currently mounted only inside the authed shell, it moves up.

## Testing

Only tests that fail when real logic breaks:

1. Go: `AUTH_CANCEL` closes the pending callback server, and a following
   `AUTH_START` succeeds.
2. Go: the callback failure page renders the code and the escaped detail for
   each case (table test).
3. Main: crash-report building redacts token patterns and the home path,
   and the issue URL stays under `MAX_ISSUE_URL_BYTES` by trimming the oldest
   lines (table test).
4. Renderer: the auth screen-state function maps each auth slice and
   connection combination, including a disconnect while waiting →
   interrupted.

No React component tests. Check every screen by hand in the running app,
including a forced slow start, a killed engine, a cancelled sign-in, a denied
sign-in, and a 5-minute timeout. For the timeout, use a test-only shorter
`-auth-callback-timeout` flag if one is cheap to add; otherwise check it by
reading the code.

## Out of scope

- Sync start and progress: spec 2.
- GitLab / Gitea sign-in.
- Embedding the Geist font in the callback page. System fonts were approved.
