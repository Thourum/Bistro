# Bistro product surface design

**Issue:** BIS-13 — Design keyboard-first review workflow and product surface
**Status:** design handoff
**Audience:** app implementation, product, website/brand work

Bistro is a local-first code review cockpit. The user should feel that every PR is already on their machine, every review action is one key away, and every state is explicit enough to trust without opening a browser.

This spec consolidates the core product surface across the desktop app and the public website. It is implementation-aware: it assumes the Go sync engine owns SQLite writes, Electron renders from the local WebSocket API, and keyboard bindings live as remappable app config.

---

## Design principles

1. **Keyboard first, not keyboard only.** Mouse affordances exist, but the primary path is visible focus + single-key actions.
2. **Local-first trust.** Always distinguish cached, syncing, queued, failed, and rate-limited states. Never make the user guess whether an action left their machine.
3. **Calm density.** Reviewers need more signal, not more decoration. Use compact rows, hard separators, square focus, and stable chrome.
4. **Action before navigation.** The interface should keep the next likely review action within one key: open, scan, comment, approve, request changes, move on.
5. **App UX and website UX are separate.** The app is a fast review tool. The website explains the tool, proves trust, and gets a developer to install it.

---

## App information architecture

### Persistent chrome

- **Top toolbar:** repo scope, search/filter summary, command palette hint, sync state.
- **Main review surface:** inbox, PR overview, diff, comments, and status panes.
- **Bottom system bar:** keyboard legend, current focus, pending queue count, sync/rate-limit status, last local update.

The bottom bar should be present in every authenticated screen. It is the user's trust rail: `j/k MOVE // : COMMAND // 3 QUEUED // SYNCED 2M AGO`.

### Primary screens

| Screen | Purpose | Primary keys | Notes |
| --- | --- | --- | --- |
| Login / provider connect | Explain local-first auth and connect GitHub | `enter`, `?` | Token storage copy must name OS keychain. |
| Inbox | Triage all review work | `j/k`, `enter`, `/`, `f`, `:` | Default screen after launch. |
| PR overview | Decide whether to inspect deeply | `enter`, `d`, `a a`, `r r`, `c` | Shows summary, CI, reviewers, files, threads. |
| Diff review | Read code and act on lines | `h/j/k/l`, `n/p`, `[ / ]`, `c`, `r`, `a a` | Full-screen variant for deep review. |
| Command palette | Find any action by intent | `:`, type, `enter`, `esc` | Also teaches shortcuts. |
| Settings | Bindings, repos, sync behavior, appearance | `cmd+,`, `:settings` | Settings are local and editable. |
| Help overlay | Discover current context bindings | `?`, `esc` | Generated from real keybinding config. |

---

## Inbox hierarchy

The inbox should answer: **What needs my review attention now?** It should not mirror GitHub's flat PR list.

### Default lanes (shipped — Phase 5, 2026-07-18)

First-match-wins, in this order. Full rules and edge cases:
`docs/superpowers/specs/2026-07-18-phase5-inbox-triage-polish-design.md` §5.1.

1. **Needs Action** — PR is open AND: `changes_requested` on a PR you authored,
   OR you're in `requested_reviewers`, OR you reviewed it and the author has
   pushed since (`head_sha != your last_reviewed_sha`).
2. **Waiting on Author** — PR by someone else, open, you reviewed or
   commented on it, and the author hasn't pushed since your review.
3. **Your PRs** — `pr.author === viewer`.
4. **Your Code** — `touches_you` (Phase 4 expertise signal; lane renamed,
   field unchanged).
5. **Mentioned** — `@your-login` appears in the PR body or comments.
6. **FYI** — everything else.

Jump directly to any lane with `g 1`…`g 6`; `Shift-R` marks the current lane's
filter-visible PRs read. The repo-grouped view remains available via the
existing grouping toggle (`:view groups` / Settings).

### Row model

Each PR row should fit in two dense lines plus status atoms:

```text
▌ NEEDS_ACTION  fix auth refresh race (#842)              +126 / -34  8 FILES
  api/auth · @mira · 12m ago · CI PASS · 3 THREADS · YOU REQUESTED
```

Required signals:

- repo and PR number
- author and recency
- review reason: `YOU_REQUESTED`, `MENTIONED`, `AUTHOR_REPLIED`, `CI_CHANGED`, `WATCHED`
- CI aggregate
- unresolved thread count
- local sync freshness
- queued action count when present

### Inbox keyboard behavior

- `j/k` moves row focus, not scroll position alone.
- `enter` opens selected PR and preserves inbox selection.
- `space` toggles the selected repository group in grouped view.
- `/` filters the current lane locally first.
- `f` opens filter chips; filter keys are visible in-place.
- `g 1`…`g 6` jump to lane 1–6 (shipped, replaces the earlier `g n`/`g w`/`g a`/`g f` sketch).

### Empty states

Empty inbox states should reinforce speed and trust, not celebration.

- **No PRs watched:** `NO_REPOS_WATCHED` — show repo picker and `:watch repo` hint.
- **No needs action:** `NEEDS_ACTION CLEAR` — show next lane shortcut and last sync time.
- **Search empty:** `NO LOCAL MATCHES` — show active query and clear key.
- **Offline empty cache:** `NO CACHED PRS` — explain that Bistro needs one successful sync before offline review.

---

## Keyboard-first review workflow

### Happy path: review and approve

```text
launch → inbox focused
j/k     select PR
enter   open overview
enter   open diff or focused first file
j/k     scan lines
n/p     move files
c       leave line comment when needed
a a     approve with double-tap confirmation
esc     return to inbox, selection moves to next actionable PR
```

### Request changes path

- `r r` opens request-changes composer with focus in the summary field.
- Inline comments remain anchored in the diff and appear in a pending review tray.
- `cmd+enter` submits the review; `esc` closes composer without discarding until confirmed.
- Bottom bar must show `DRAFT_REVIEW: 3 COMMENTS` while comments are staged.

### Comment path

- `c` on a diff line opens an inline composer below that line.
- `r` on an existing thread opens the reply composer inside the thread.
- `cmd+enter` / `ctrl+enter` queues the comment.
- The comment appears immediately with a `QUEUED` status square until the Go engine confirms sync.

### Safety confirmations

Use double-tap for irreversible or high-signal actions:

- `a a` approve
- `r r` request changes
- `m m` merge, if Bistro ever supports merge
- destructive comment deletion should use `del del` or a typed command, not a single key

Confirmation text should be plain: `PRESS a AGAIN TO APPROVE`. No modal unless the action needs a message.

---

## Command palette and action model

The command palette is both execution surface and discoverability layer. It should answer: **What can I do from here, and what key would have done it faster?**

### Palette behavior

- Open with `:` from any screen.
- Search actions, PRs, repos, settings, filters, and docs pages.
- Rank context-valid actions first.
- Show the shortcut at the right edge of every command row.
- Disabled actions remain searchable with the reason: `requires selected diff line`, `offline queued`, `not supported by provider`.

### Action schema

Every action should be modeled as data, not scattered button handlers:

```ts
type BistroAction = {
  id: string
  title: string
  scope: 'global' | 'inbox' | 'pr' | 'diff' | 'thread' | 'settings'
  keys?: string[]
  enabledWhen: string
  run: 'local-route' | 'websocket-command' | 'external-url' | 'settings-update'
  safety?: 'none' | 'double-tap' | 'composer' | 'confirm-dialog'
}
```

Implementation handoff: keep keybindings and command rows generated from the same action registry so `?`, footer hints, settings remaps, and palette shortcuts cannot drift.

### Initial command set

- `open selected PR`
- `approve PR`
- `request changes`
- `comment on current line`
- `reply to thread`
- `resolve thread` / `unresolve thread` when provider support exists
- `copy PR URL`
- `open in browser`
- `filter: needs action`
- `filter: waiting on author`
- `watch repository`
- `sync now`
- `show queued actions`
- `settings: keybindings`
- `settings: repositories`

---

## PR overview ergonomics

The overview is the stop between inbox triage and full diff reading. It should help the reviewer choose the cheapest next action.

### Overview layout

1. **Header:** title, repo, branches, author, review reason, CI aggregate.
2. **Action strip:** approve, request changes, comment, open diff, open browser. Buttons show keys, not just labels.
3. **Review readiness:** unresolved threads, requested reviewers, CI, merge conflicts, stale branch, queued local actions.
4. **File summary:** changed files grouped by package/folder with additions/deletions.
5. **Thread summary:** unresolved first, human before bot, current patchset before outdated.

### Overview states

- **Diff not generated yet:** show metadata and `GENERATING_LOCAL_DIFF…`; keep actions that do not need diff enabled.
- **CI pending:** show pending as neutral amber, not blocker red.
- **Queued approval/comment:** show action as local pending and keep route usable.
- **Provider unsupported action:** show disabled command with provider note rather than hiding it.

---

## Diff and comment ergonomics

The diff is the main work surface. It should preserve line focus, reduce scroll disorientation, and make comments feel anchored to code.

### Diff layout

- Sticky file header: path, file status, additions/deletions, comments, current hunk.
- Left gutter: old/new line numbers, diff markers, thread markers.
- Main code column: monospace, syntax highlighted, virtualized.
- Inline thread cards: square, dense, visually nested under the exact line.
- Right rail or mini-map: files + comment density for large PRs.

### Navigation rules

- `j/k` move visual line focus.
- `ctrl+d/u` page through viewport.
- `n/p` move files.
- `[ / ]` move hunks.
- `0/$` move within the current line horizontally when line wrapping is off.
- `esc` returns to the previous surface and preserves diff position.

### Comments

- Inline composers should never cover code above the anchor line.
- Thread cards show author, timestamp, body, status, and available key actions.
- Status vocabulary: `QUEUED`, `SYNCING`, `POSTED`, `FAILED`, `OUTDATED`, `RESOLVED`.
- Bot comments should be visually quieter than human comments and collapsible by command, but not hidden by default in MVP.
- Outdated comments stay near their best re-anchor when possible; if not possible, show them in the thread summary with a clear `ANCHOR_LOST` label.

### Large PR behavior

- Render first meaningful diff content before comments if comments are slower.
- Preserve focus even when virtual rows mount/unmount.
- File list should expose changed-file counts and comment hotspots.
- Never block scrolling on network; the local cache should remain navigable.

---

## Loading, empty, and error states

### Loading

Use delay thresholds so Bistro feels instant:

- under 200ms: no visible loader
- 200–800ms: subtle text status in the relevant pane
- over 800ms: skeleton rows or diff placeholders
- over 5s: actionable error or offline explanation

Preferred copy:

- `SYNCING REPOS…`
- `GENERATING LOCAL DIFF…`
- `LOADING CACHED COMMENTS…`
- `REPLAYING 2 QUEUED ACTIONS…`

### Offline and queued actions

Offline is a supported mode, not an error.

- Read-only cached data remains normal contrast.
- Actions that can queue remain enabled and display `QUEUED` after submission.
- Actions that require live provider confirmation explain why: `requires GitHub connection`.
- System bar always shows `OFFLINE // LAST_SYNC 2H AGO // 2 QUEUED`.

### Error states

Error messages should include: what failed, whether local data is safe, and the next action.

| Situation | Message | Action |
| --- | --- | --- |
| GitHub rate limit | `RATE_LIMITED // USING CACHE` | show reset time, allow local review |
| Diff generation failed | `LOCAL_DIFF_FAILED` | retry, open logs, open browser fallback |
| Comment post failed | `COMMENT_NOT_POSTED // KEPT IN QUEUE` | retry, edit, discard |
| Auth expired | `GITHUB_AUTH_EXPIRED` | reconnect provider |
| WebSocket disconnected | `SYNC_ENGINE_DISCONNECTED` | reconnect, view engine logs |

No stack traces in UI chrome. Detailed diagnostics belong behind `:open logs`.

---

## Website and brand UX

The website should not mimic the app chrome one-to-one. It should sell the promise while preserving Bistro's calm technical voice.

### Website goals

1. Explain the wedge: AI-era PR volume broke browser review workflows.
2. Prove local-first trust: no server, local SQLite cache, OS keychain token storage, local diff generation.
3. Show keyboard speed: concrete workflow snippets, not vague “10x” claims.
4. Convert developers to install, star, or join beta.

### Home page structure

1. **Hero:** `Review pull requests at the speed of thought.` Screenshot-first, with `j/k`, `enter`, `a a` visible in chrome.
2. **Problem:** PR volume, force-pushes, bot comments, browser latency.
3. **Product proof:** local sync engine, local diffs, offline queue, command palette.
4. **Workflow strip:** inbox → overview → diff → queued approval.
5. **Trust section:** no hosted server, token in keychain, data on device.
6. **Keyboard reference preview:** searchable binding table excerpt.
7. **CTA:** download/source instructions while installers are pending.

### Visual direction

- Use the established Minimal Light direction from the design system for public surfaces.
- Square corners, hairline borders, one blue accent, near-white surfaces.
- Screenshots and product chrome are the primary imagery. Avoid abstract illustrations.
- Website copy can use sentence case; app/system chrome remains uppercase mono.

### Website empty / unavailable states

- Download unavailable: explain source install and release path.
- Docs page missing: route to README/development docs, not a generic 404.
- Screenshot unavailable: show a lightweight product wireframe with real labels.

---

## Implementation follow-ups

Create or track these as separate issues when implementation starts:

1. **Action registry:** unify command palette, help overlay, footer hints, and keybinding settings around one action model.
2. ~~**Inbox lane classifier**~~ — shipped (Phase 5, 2026-07-18): six-lane taxonomy, see "Default lanes" above.
3. **Queued action UI:** standardize `QUEUED`, `SYNCING`, `FAILED`, and retry/discard controls for comments and reviews.
4. **Diff focus model:** ensure virtualized diff rows preserve line focus across navigation and remounts.
5. **Website IA refresh:** split marketing copy from app UX docs and update website plan around local-first trust proof.
6. **Provider capability labels:** surface unsupported provider actions in palette/help with reasons instead of hiding them.

---

## Acceptance checklist for BIS-13

- [x] User-facing UX recommendations for inbox, diff, comments, actions, shortcuts, and states.
- [x] Calm, fast, local-first, developer-tool-native feel.
- [x] Clear separation between app UX and website/marketing UX.
- [x] Implementation-aware follow-ups for future build work.
