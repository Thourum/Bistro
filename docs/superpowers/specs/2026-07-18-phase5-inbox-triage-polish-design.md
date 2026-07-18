# Phase 5 — Inbox & Triage Polish: Design

**Date:** 2026-07-18
**Status:** Approved by user (brainstorming session)
**Scope:** All four ROADMAP Phase 5 sub-areas: 5.1 Reviewer Inbox, 5.2 Diff polish, 5.3 Command palette polish, 5.4 Help/discoverability.

## Decisions log (user-approved)

- Lane taxonomy: revised 6 lanes (below), replacing both the ROADMAP and DESIGN.md taxonomies. ROADMAP/DESIGN.md to be updated to match.
- Read state lives in SQLite via the Go engine (new migration), not renderer-local.
- `/prs` evolves into the Inbox (no new route). Layout: sectioned single list (mockup option A).
- Lane assignment computed in the renderer (pure TS function over list-payload fields).
- The `touches_you` wire/DB field name stays; only the lane label/id uses the new name.
- 5.2 is built by migrating the diff surface to `@pierre/diffs` `CodeView` first, then the polish features become configuration. Image/binary diffs deferred (placeholder only).
- 5.4: first-run tour only. "Did you know?" tooltips are won't-do (mark in ROADMAP).
- `:open in editor`: verify head SHA locally; offer checkout or open-current-version when missing.

---

## 5.1 Reviewer Inbox

`/prs` becomes a prioritised, lane-grouped landing screen. One scrolling list divided by six uppercase lane headers showing counts at all times (`1 NEEDS ACTION — 2`). Empty lanes render the header plus a dimmed "clear" line. The existing repo-grouped view remains available via the current grouping toggle.

### Lanes (first-match-wins, in this order)

| # | Lane | Rule |
|---|---|---|
| 1 | **Needs Action** | PR is **open** AND: `changes_requested` on a PR you authored, OR you're in `requested_reviewers`, OR you reviewed it and the author has pushed since (`head_sha != your last_reviewed_sha`) |
| 2 | **Waiting on Author** | PR by someone else, **open**, you reviewed or commented on it (`approved_by`/`changes_requested_by` = you, or `viewer_commented`), and the author has NOT pushed since your review |
| 3 | **Your PRs** | `pr.author === viewer` |
| 4 | **Your Code** | `touches_you` (existing Phase 4 field; lane renamed, field name unchanged) |
| 5 | **Mentioned** | `@your-login` appears in PR body or comments (new `mentioned` flag) |
| 6 | **FYI** | Everything else |

Ordering notes:

- Lanes 1 and 2 require `state === 'open'`: a closed/merged PR carrying stale review-request or changes-requested signals never outranks live work (it falls through to Your PRs / Your Code / FYI).
- "Author pushed since your review" is detected via the existing `last_reviewed_sha` table (written on every successful review action): when `head_sha` no longer equals your recorded review SHA, the PR moves from lane 2 back to lane 1 — an author responding with commits resurfaces as actionable even when GitHub does not re-request review. (An author responding with *comments only* does not re-lane; accepted limitation.)
- "Your PRs" is checked before "Your Code" so your own PRs don't land in lane 4.
- "Waiting on Author" requires `author !== viewer`, so your own PRs never match it.
- When GitHub re-requests your review after you reviewed, `requested_reviewers` contains you again → the PR returns to lane 1.
- **Unhydrated (list-only) PR caveat:** `viewer_commented` and comment-borne mentions come from locally synced comments; list-only rows (only on very active repos, >100 open PRs) have none until hydrated, so such a PR can sit in FYI despite your participation. Bounded: hydration happens on selection and signals recompute on the next list push. Disclosed here as a product trade-off, not just a display delay.

### Lane assignment (renderer)

- New `electron/src/renderer/utils/lanes.ts`: pure `laneForPR(pr, viewerLogin) → 1..6`, exhaustively unit-tested. First-match-wins in the table order above.
- Inputs are all present in the PR list payload: `state`, `review_status`, `changes_requested_by`, `approved_by`, `requested_reviewers`, `author`, `touches_you`, plus the new fields below.
- Sort within a lane: `updated_at` DESC (existing default).
- `useVisiblePRs` remains the single canonical list source; lane grouping is a presentation layer over it, and `selectedPRIndex` continues to address the flat visible list.

### New Go-side signals

1. **`mentioned` flag** — migration 033 adds a `mentioned` column to `prs`. Computed at classify time by scanning the locally-synced PR body + comments for `@<viewer-login>` (no new GitHub API calls). Shipped in the PR list payload.
2. **`viewer_commented` flag** — computed field in the list payload (EXISTS subquery against the local `comments` table for the viewer's login). No migration. Caveat (accepted): list-only/unhydrated PRs have no comments synced, so the flag is false for them until hydration; such PRs sit in FYI.
3. **`reviewed_head_stale` flag** — computed field in the list payload: true when a `last_reviewed_sha` row exists for (pr, viewer) and its `sha != prs.head_sha`. Powers the lane-1 "author pushed since your review" rule. No migration (`last_reviewed_sha` shipped in migration 014, written on every successful review action).
4. **Read state** — migration 034 adds `pr_read_state (pr_id, login, last_read_at)`. A PR is unread when it has no row or `updated_at > last_read_at`. New WS message `MARK_PRS_READ {pr_ids}`: Go upserts rows and pushes the updated list. Sent by `Shift-R` (all PRs in current lane) and automatically when a PR is opened. Unread rows render bold with a dot prefix; read rows dimmed.

Go owns all SQLite writes, per convention.

### Keyboard

All bindings are pure config in `keybindings.ts` (sequence bindings are already supported):

- `g 1` … `g 6` — jump to first row of lane N.
- `j`/`k` — unchanged; walk the flat visible list across lane boundaries.
- `Shift-R` — mark read all *filter-visible* PRs in the current lane (the lane rows the user can see, after search/repo filters). "Current lane" = the lane containing the selected row; with no selection (empty list) the binding is a no-op.

---

## 5.2 Diff polish (via CodeView migration)

### Step 0 — migrate `PierreDiffView` to `CodeView`

`PierreDiffView.tsx` (1114 lines) currently hand-rolls virtualization with TanStack Virtual around per-file `PatchDiff` components — the architecture the library's own authors describe as a stopgap. `@pierre/diffs` 1.2.11 (installed) ships `CodeView`: a virtualization-first review surface with native scrolling (no blanking), deferred worker-pool syntax highlighting, and shared options state (a split/unified toggle is one state update, not a per-file config rewrite).

- **Verification spike first:** render a real PR diff with inline comment annotations and keyboard scrolling under `CodeView` before committing to the migration. If the spike fails on a hard requirement (annotations, keyboard nav), fall back to the minimal PatchDiff plan (props + collapse wiring on the existing view).
- The migration deletes our TanStack Virtual scaffolding and splits `PierreDiffView.tsx` into focused modules (the file is over the 800-line ceiling regardless).
- Comment threads/annotations, file-jump keys (`n`/`p`), and the diff cursor re-wire against CodeView's API.

### Features (mostly configuration after step 0)

| Feature | Mechanism |
|---|---|
| Side-by-side toggle | `Shift+S` toggles `diffStyle: 'unified' \| 'split'` (library prop). Persisted in settings as the default view mode. Plain `s` stays bound to the existing `diff.sinceLastReview` range diff — unchanged. |
| Word-level intra-line diff | `lineDiffType: 'word'` + library's `maxLineDiffLength` cap. On by default; no UI. |
| Whitespace-only collapse | Custom: pure `isWhitespaceOnlyChange(hunk)` over parsed patch data (compare `-`/`+` lines after whitespace normalization). `w` toggles hiding those hunks behind a one-line "N whitespace-only lines hidden" marker. The library has no ignore-whitespace option (verified against 1.2.11 typings). |
| Generated-file collapse | Custom detection: constant pattern list (`*.lock`, `pnpm-lock.yaml`, `go.sum`, `*_generated.*`, `dist/`, linguist-generated conventions), not user-configurable. Collapse rendering uses the library's `collapsed` option — the file header with its diff stats stays visible, so nothing is hidden from discovery, only from default scroll. Rationale: these files are machine-written and their review signal is "it changed + by how much", which the header already shows; one keypress expands when the content matters. Configurability deferred until a real pattern gap shows up (YAGNI). Manual expand/collapse per file. |
| Image/binary diffs | Deferred. Binary files render a placeholder row: "binary file — `o` opens in browser". `o` here targets the specific file's blob view on the PR (GitHub `…/pull/N/files#diff-<file-anchor>`), distinct from the PR-level `list.openBrowser`/`:open in browser` action which opens the PR page. |

---

## 5.3 Command palette polish

- **`:open in browser`** — palette command wired to the existing `list.openBrowser` action path (`o` keybinding + `openExternal` IPC already exist; wiring only).
- **`:open in editor`** — opens the current diff file at the current line.

  **Current file/line model** (defined; no new cursor state invented): the *file* is the one the diff view is focused on (the `n`/`p` file navigation target). The *line* is the selected inline-comment line when an inline comment is selected; otherwise the first line of the file's first hunk (new-side line number); fallback line 1. This reuses existing diff-view state only — the diff surface has no general line cursor, so "the line you're reading" is not knowable. The palette entry is therefore worded **"Open file in editor"** (not "at current line") and its description says "at the selected comment's line when one is selected"; the tour does not promise line-level precision.

  **Flow:**
  1. Look up the repo's local path (`repo_location`, migration 026). No local path → error toast.
  2. Go WS request compares `git rev-parse HEAD` in that worktree against the PR head SHA (and reports whether the SHA object exists locally via `git cat-file -e`).
  3. **HEAD == PR head SHA** → open `$EDITOR` at `file:line` using the `+line` CLI convention (`code -g file:line`, `vim +line`, fallback `$EDITOR file`), launched from the Electron main process.
  4. **HEAD != PR head SHA** (whether or not the object exists locally) → dialog: **"PR revision not checked out"** with three choices: *Check out PR head* (Go runs `git checkout <sha>`, preceded by `git fetch` when the object is missing locally; refuses on a dirty worktree with an error toast), *Open current version* (open the local file as-is, with an explicit "may differ from the PR revision" warning in the dialog body), *Cancel*. Opening the working-tree file never happens silently on a mismatched HEAD.

  **Why checkout leaves HEAD moved (deliberate):** the reviewer's stated intent when choosing *Check out PR head* is "put my working copy on this PR so I can inspect/run it" — jumping between branches is normal git workflow for this user (a developer with a clean worktree), and silently restoring the previous checkout afterwards would be more surprising than leaving them where they asked to go. The dialog body states the current branch being left (e.g. "This moves your checkout from `main` to the PR head (detached)."). Non-disruptive alternatives (temp worktree) are out of scope for v1.

---

## 5.4 First-run tour

- 5-screen overlay, new `OverlayKind: 'tour'`: (1) Inbox & lanes, (2) `j`/`k`/`enter` navigation, (3) diff review keys, (4) comment/approve actions, (5) command palette & `?` help.
- Keys that are route-dependent are labeled with their context on every screen (e.g. screen 1: "`Shift-R` — mark lane read *(in the inbox)*"; screen 4: "`Shift-R` — request changes *(on a PR)*"), so the same key appearing twice reads as deliberate, not contradictory.
- Key hints render from the actual `keybindings.ts` config so user overrides display correctly — no hardcoded key names.
- Shows once (settings flag), skippable with `Esc`, re-runnable via `:tour`.
- "Did you know?" tooltips: won't-do; ROADMAP updated accordingly.

---

## Error handling

- `MARK_PRS_READ` failures: read state is local-only (not a GitHub write), so it does not use the offline action queue. On WS failure the mark is dropped and the PR stays unread until next opened — logged, no user-facing error. Successful writes trigger a `PRS_READ_CHANGED` push; the list refetches on it.
- `:open in editor`: every failure path (no local path, dirty worktree, checkout failure, editor launch failure) surfaces a user-facing toast with the specific reason; details logged.
- Checkout is the only state-mutating git operation added; it refuses to run on a dirty worktree.

## Testing

- **Unit (TS):** `laneForPR` (exhaustive over lane rules and ordering), `isWhitespaceOnlyChange`, generated-file pattern matching, editor CLI argument construction.
- **Unit (Go):** migration 033/034 schema tests (existing `db_test.go` pattern), `mentioned` computation, `viewer_commented` subquery, `MARK_PRS_READ` handler, cat-file/checkout handlers (incl. dirty-worktree refusal).
- **Component:** lane headers + counts render, `Shift-R` mark-read flow, unread/read row styling, tour overlay show-once/re-run.
- **Manual/spike:** CodeView renders a large real PR with annotations; side-by-side and word-diff verified visually; `pnpm bench:large-diff` compared before/after migration.

## Out of scope

- Image diff rendering (placeholder only) — follow-up.
- "Did you know?" tooltips — won't-do.
- Renaming the `touches_you` DB/wire field — lane label only.
- Mute/archive lanes, configurable generated-file patterns, usage tracking.
