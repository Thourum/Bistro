# First run, specs 2–4: onboarding steps and first-run rail — design

Status: ruled by the controller on 2026-10-24. The user set the goal "finish
implementing the whole thing, Step 1, Step 2 and First-run tour according to
the design files" and asked not to be consulted further. Open questions are
settled here as **Rulings**, each with its reason.

This spec continues `2026-10-24-first-run-loading-auth-design.md`. It covers
the remaining three parts of that program in one plan:

1. **Step 1/2 — choose what to sync** (design frames 1o–1s)
2. **Step 2/2 — style + practice** (frames 1k–1n, 1w)
3. **First-run rail** over the live inbox (frames 1t–1v). The 5-screen modal
   (1x) is dropped; `:tour` reopens the rail.

The design bundle is git-excluded at `design/onboarding-flow/`. Rendered frames
are in `/tmp/bistro-design-shots/<frame>.png` (regenerate with
`/tmp/bistro-design-shots/shoot.mjs` if missing). The design is a layout and
look-and-feel reference; its action names, keys and some copy are
placeholders. Where it disagrees with this spec, this spec wins. Keys always
come from the active binding map, never hard-coded.

**Ruling — step order.** The user swapped the design's steps: choosing what
to sync comes first, so the sync runs while the user practises and the inbox
is ready when onboarding ends. Footers read `1 / 2` then `2 / 2`; step 2's
button reads **Finish**.

## Current code (what changes)

- `pages/OnboardingPage.tsx` has two steps, `'navigation'` then `'devFolder'`.
  `finish()` sets `appearance.onboardingComplete`. Continue is
  `onboarding.continue` (Space) through `confirmingAction`.
- The dev-folder step validates, saves `repositories.devFolderPath`, and sends
  `SCAN_DEV_FOLDER`. The scan matches local clones to repos by remote URL and
  writes `repos.local_path`; it never changes `watched`.
- The practice sandbox (`utils/onboardingSandbox.ts`, `pages/OnboardingSandbox.tsx`)
  has 8 tasks over fake data. Hotkeys reach it through `useAppHotkeys` and
  `SANDBOX_ACTION_IDS`.
- `TourOverlay` auto-opens while `appearance.tourComplete` is false; `:tour`
  replays it.
- Engine: `REQUEST_REPO_LIST` lists the user's GitHub repos (watched stays 0);
  `TOGGLE_WATCH_REPO {repo_id, watched?}` sets the flag and triggers that
  repo's sync on false→true. Nothing syncs right after sign-in; the first sync
  waits for the ticker. Inbox discovery (`involves:USER`) ignores `watched`.
- Bug: `triggerSync` in `handler/repolocation.go` sends `"repo_locations"` as a
  repo id, which `syncSingleRepo` rejects, so a scan never triggers a sync.

## Engine changes

1. **Sync right after sign-in.** When `AUTH_SUCCESS` is broadcast, push `""`
   (sync all) to the sync trigger. Onboarding then runs while the inbox fills.
2. **Fix the scan trigger.** `triggerSync` pushes `""` instead of
   `"repo_locations"`. Add a regression test that asserts the pushed value.
3. **Review queue.** New WS request `REQUEST_REVIEW_QUEUE` → reply
   `REVIEW_QUEUE_READY`:

   ```json
   {
     "requested": [{ "repo_id": "github:owner/repo", "count": 3 }],
     "recently_reviewed": ["github:owner/repo"]
   }
   ```

   - `requested`: GitHub search `is:pr is:open archived:false
     review-requested:USER`, grouped by repo, sorted by count descending, then
     name.
   - `recently_reviewed`: search `is:pr reviewed-by:USER updated:>=<today-30d>`,
     distinct repos, most recent first, at most `RECENT_REVIEW_REPO_LIMIT = 10`.
   - One page of `per_page=100` each. **Ruling:** no pagination; a reviewer with
     more than 100 open requests still gets a correct ON set for the top 100,
     and the inbox sync is unaffected.
   - Every returned repo is upserted into `repos` (watched unchanged), so the
     renderer can toggle it with the existing `TOGGLE_WATCH_REPO`.
   - Failure replies `{ "error": "<message>" }`.
4. **Cancel a scan.** New WS message `SCAN_CANCEL`. The handler cancels the
   context of the in-flight `SCAN_DEV_FOLDER`; the scan returns with
   `partial: true` and the clones matched so far are kept. A no-op when nothing
   is scanning. **Ruling:** no per-directory progress events; the UI shows an
   indeterminate scanning state. The scan is capped at 30 s already.

**Ruling — what ON/OFF means.** ON is the existing `watched` flag: Bistro syncs
every open PR in that repo. OFF repos still show PRs that involve you, because
inbox discovery is unchanged. This keeps existing users' inboxes intact and
makes the toggle honest. The table's footnote says so (see Copy).

## Step 1/2 — choose what to sync

Layout follows 1q: a left card and a right repositories panel, stacking below
`md` with the panel second.

### Left card

- Icon tile, "Start from your review queue", and: "Bistro asks GitHub which
  PRs are waiting on you. Repos with requests are switched on. The rest stay
  off until you need them."
- Mono note: "Bistro only fetches into `refs/bistro/*`. It never touches your
  branches, tags, or files. Repos can be removed later."
- Divider, then **Reuse local clones // optional**: "Point Bistro at your
  projects folder and it fetches into clones you already have instead of
  making new ones."
  - No folder: **Choose folder** button with its key hint.
  - Scanning (1p): "Scanning `<folder>`", an indeterminate bar, **Stop** with
    the stop key (`Esc`).
  - Done: the folder (home shown as `~`), "`N` of `M` matched", and **Choose
    another folder**.
  - Error (1s): inline destructive alert "Couldn't read `<folder>`: `<reason>`.
    Repos will clone on first open instead." plus **Choose another folder**.
    The table stays usable.
- If `repositories.devFolderPath` is already set (from an earlier run), show
  the Done state immediately with its match count from the repo list.

### Repositories panel

Header row: `REPOSITORIES` left; right shows `READING…` while loading, else
`N PRS NEED YOU // M REPOS` when anything needs you, else `0 WAITING ON YOU`.

**Loading order — Ruling (user: "start by showing only local ones, lazy load
GitHub").**
1. On mount, request the repo list from the local cache (`REQUEST_REPO_LIST`
   reply as it exists) and show repos that have a local clone immediately.
2. In parallel, send `REQUEST_REVIEW_QUEUE`. Until it answers, the panel shows
   the local rows plus a reading line ("Reading your review requests…").
   With no local rows, show the centered reading state of 1o.
3. When the queue arrives, merge: requested repos first, then the rest.

**Rows.** Columns: Sync · Repo · Needs you · Clone.
- Sync is an `ON` / `OFF` tag (filled square + accent text / hollow square +
  muted), not a checkbox.
- Needs you shows the request count, accent-coloured when > 0.
- Clone shows the local path (home as `~`), `Matching…` while a scan runs and
  the repo has no path yet, or "Clones on first open".
- **Ruling:** the design's Open PRs column is dropped. There is no cheap
  per-repo count, and a search per repo would slow the step down.
- **Ruling (user: "needs_you optional … show it only if we find something
  actually needs user").** The Needs you column and the `WAITING ON YOU (n)`
  / `NOTHING WAITING (n)` group headers appear only when at least one repo has
  a request. Otherwise rows are one ungrouped list.

**Defaults.** Repos with requests are ON. Every other repo starts at its
current `watched` value (OFF for a new user). The user's changes are local
until Continue.

**Nothing waiting (1r).** When the queue reports no requests: heading
"Nothing is waiting on you right now", body "When someone requests your
review, Bistro switches that repo on for you. You can also turn on repos you
review often.", then the `recently_reviewed` repos under `REVIEWED IN THE LAST
30 DAYS (n)`, all OFF, followed by the remaining repos. **Ruling:** the copy
"switches that repo on for you" describes the inbox, which already shows new
requests; Bistro does not flip `watched` automatically. Copy becomes: "When
someone requests your review, it shows up in your inbox. You can also turn on
repos you review often."

**Queue error.** Inline alert in the panel: "Couldn't read your review
requests: `<message>`." The table still lists cached and local repos; Continue
stays enabled.

**Toolbar.** Filter input ("Filter repos…"), `n of m selected`, **All** and
**None** with key hints. Filter matches `owner/repo` substrings,
case-insensitive.

**Footer of the panel.** Key hints for move, toggle, filter, then the
footnote: "PRs that involve you always reach your inbox. ON also syncs every
open PR in the repo. Change this any time in Settings → Repos."

**Continue.** For each repo whose desired ON/OFF differs from its `watched`,
send `TOGGLE_WATCH_REPO {repo_id, watched}`; then advance to step 2. Requests
are fire-and-forget; failures toast. Continue is disabled only while the
first repo-list reply is pending (the "Reading" state with no rows).

**Keys** (new catalog actions on route `onboarding`, active only on step 1):

| Action | vim | emacs | standard |
|---|---|---|---|
| `onboarding.repoDown` | `j` | `Control+n` | `ArrowDown` |
| `onboarding.repoUp` | `k` | `Control+p` | `ArrowUp` |

Shared in every style: `onboarding.repoToggle` `x`, `onboarding.repoAll` `a`,
`onboarding.repoNone` `n`, `onboarding.repoFilter` `/`,
`onboarding.chooseFolder` `o`, `onboarding.stopScan` `Escape` (only while
scanning). **Ruling:** shared single letters keep the step learnable before
the user has picked a style (that happens in step 2).

Shared keys go in `SHARED_KEYS`; move keys go in the per-style tables. The
`keymaps.test.ts` collision guard must stay green. In the filter input, keys
are ignored (existing `ignoreInputs`), except Escape, which blurs the input.

## Step 2/2 — style + practice

Layout follows 1k: left column (picker + Try it), right column (practice
pane), stacking below `md` (1n).

### Style picker (left, top)

- Label `NAVIGATION_STYLE` with the next/prev style key hints.
- Vertical cards, one per style (vim, emacs, standard). The active card is
  expanded: filled square, name, `ACTIVE`, the one-line description, a row of
  the style's key chips. Others collapse to a hollow square, name and
  description. Emacs' expanded card adds the Windows/Linux prefix note from
  the existing picker copy.
- Reuse `NAVIGATION_STYLE_CARDS` data; restyle `NavigationStylePicker` with a
  `layout="vertical"` prop (Settings keeps its current layout).

### Try it (left, below the picker)

- Label `TRY_IT` and `done / total` on the right, turning success-coloured with
  a filled square at total.
- A 3-tab strip: `01 // d/n` + stage name. A finished stage shows a check and
  success colour. Clicking a tab selects that stage; the active stage follows
  the first unfinished task otherwise.
- The stage's tasks: done (check, struck through, muted), current (`→`, 2px
  accent left border, bold, key chip filled), todo (two-digit index). Some
  tasks carry a one-line reason under the title.
- Footnote: "Practice is optional and every stage can be skipped. Nothing here
  reaches GitHub."
- At total: `PRACTICE_COMPLETE // m:ss` (elapsed since the step opened) and
  "That's the whole review loop. Press `<continue key>` to continue." Continue
  gets a focus ring.

### Stages and tasks

Every task maps to a real catalog action, so the keys shown are the keys the
user will use. **Ruling:** the design's tasks without a real action today
(visual line select, comment on selection, add draft to review, toggle split)
are dropped rather than invented; the spec stays at what Bistro can do now.

1. **Navigate** (pane: list) — Move down 3 times (`list.down`), Move up
   (`list.up`), Jump to the top (`list.top`), Jump to the bottom
   (`list.bottom`), Open the PR waiting on you (`list.open`; reason: "Needs
   Action only lists PRs where your review was requested.").
2. **Review the PR** (pane: PR) — Fold the bot comments (`detail.dismissBots`),
   Undo the fold (`detail.undoDismiss`), Leave a general comment
   (`detail.comment`), Request changes (`detail.requestChanges`; reason: "Posts
   a review that blocks merging."), Approve instead (`detail.approve`; reason:
   "A double-tap, so you never approve by accident."), Open the diff
   (`detail.viewDiff`).
3. **In the diff** (pane: diff) — Next file (`detail.nextFile`), Next comment
   (`detail.nextComment`), Reply to a thread (`detail.reply`), See what changed
   since your last review (`diff.sinceLastReview`; reason: "Bistro remembers
   the commit you last reviewed."), Back to the PR (`global.escape` or the
   existing back action the sandbox uses today).

The plan confirms each action id against `utils/keybindings.ts` and adjusts
names that differ; if one is missing it drops that task and records it.
Task counts follow from the list, not from the design's 5/5/7.

### Practice pane (right)

- Breadcrumb: `PRACTICE // Practice inbox / #413 / <file>` following the pane.
- Tabs `LIST · PR · DIFF`, reflecting the sandbox pane; clicking a tab is not
  a task.
- **List** (1k): five fake PRs with status square, number, title, author,
  age; selected row highlighted with a 2px accent bar.
- **PR** (1l): number + branch line, title, author/commits/+−, description,
  `CONVERSATION` with a dashed bot-fold strip ("2 bot comments folded //
  coderabbit, greptile" + undo key) and two comments, a review bar (Comment,
  Request changes, Approve, Open diff, each with its key), and a side rail
  (Checks, Reviewers, Files). An `APPROVED` badge appears after approve;
  reviewers update.
- **Diff** (1w): toolbar (`All changes` / `Since last review n`; file `x / y //
  path`), a split view of the fake hunk, and one thread with a reply hint.
  The split/unified toggle is shown but static (Ruling above).
- **Movement bar** (always visible, bottom of the pane): `MOVE // <STYLE>`,
  then down/up, top/bottom, open/back and all-keys key pairs from the active
  map, and `NEXT: <current task>` on the right (or `PRACTICE COMPLETE`).

The fake data grows to cover these panes (bots, reviewers, checks, files,
thread). It stays local and never reaches the engine.

## Onboarding footer (both steps)

`1 / 2` or `2 / 2` with a two-segment progress bar; then the live sync line;
right side "or press `<continue key>`" and the primary button (Continue /
Finish).

Sync line: a status square, then
- `SYNCING` while `syncStatus` is `syncing`, `SYNCED` when `ready`, `OFFLINE`
  when the socket is down;
- `// N NEED YOU` when the review queue reported any requests.

**Ruling:** the design's `41 PRS READ` is dropped; SYNC_COMPLETE carries no
reliable count today.

## First-run rail (frames 1t–1v)

Replaces `TourOverlay`, which is deleted. `:tour` brings the rail back with its
steps reset.

- Shown on the PR list route while `appearance.tourComplete` is false and
  onboarding is complete. A non-modal right column (fixed width, full height
  under the header); the inbox stays fully usable.
- Header: `FIRST_REVIEW // s OF 3` (or `// DONE`), a 3-segment progress bar,
  and **Hide** with its key.
- Title `N PRs need you` (Needs Action lane count) and "They're in Needs
  Action because someone requested your review. The list updates as GitHub
  does." When Needs Action is empty: "Nothing needs you yet" and "Your
  review requests appear here as they arrive."
- Steps, each with its key chip from the active map:
  1. Open the top one (`list.open`) — subline names the top Needs Action PR:
     "#1284 — @tomas requested your review 4m ago." when known.
  2. See what changed since your last review (`diff.sinceLastReview`) — "Only
     the commits pushed after your last pass." Shows `✓ PRACTICED IN
     ONBOARDING` when that sandbox task was done this session.
  3. Approve or comment (`detail.approve` / `detail.comment`) — "Posts to
     GitHub like any other review."
- A step completes when its action runs anywhere in the app (not only from
  the rail). Completion is session state in `appStore`.
- Done (1v): `That's the loop.` "Everything else is one key away. This rail
  won't open again." and **Close** with its key.
- Bottom section `EVERYTHING ELSE`: Command palette and Keyboard shortcuts
  with their keys, and "Run :tour to bring this guide back."
- Hide/Close sets `appearance.tourComplete = true` (the existing flag); the
  rail stays hidden until `:tour` clears it.
- The Needs Action lane tab gets a focus ring while the rail is visible.

**Hide key — Ruling.** New action `firstRun.hide`, key `Escape` (shared),
active only on the list route while the rail is visible and no overlay is
open. The plan checks what `global.escape` does on the list route; if both
would fire, `firstRun.hide` runs only when `global.escape` has nothing to do
(no search query, no filter), else `global.escape` wins.

## Persistence

- `appearance.onboardingComplete` and `appearance.tourComplete`: unchanged
  meaning; `tourComplete` now marks the rail dismissed.
- Repo ON/OFF persists through the engine (`watched`).
- **Ruling:** practice progress is not persisted (optional in the handoff).

## Testing

Only tests that fail when real logic breaks:

1. Go: `triggerSync` pushes `""` (regression for the no-op bug).
2. Go: `REQUEST_REVIEW_QUEUE` groups requested PRs by repo and sorts them;
   recently reviewed is distinct and capped (table test against a fake search
   server).
3. Go: `SCAN_CANCEL` cancels an in-flight scan and the reply is `partial`.
4. Go: `AUTH_SUCCESS` pushes a sync trigger.
5. Renderer: the step-1 table model — merging local, queue and cached repos,
   defaults, grouping only when something needs you, filter, all/none, and the
   diff of toggles sent on Continue (pure function, table test).
6. Renderer: the practice task reducer — task order across 3 stages, done
   counting, stage selection follows the first unfinished task.
7. Renderer: rail step completion from dispatched action ids.
8. `keymaps.test.ts` stays green with the new actions.

No React component tests. Check every screen by hand in the running app.

## Out of scope

- Visual line select, comment-on-selection, add-to-review, split toggle as
  real actions.
- Per-directory scan progress.
- Auto-watching repos when new review requests arrive.
- Persisting practice progress.
