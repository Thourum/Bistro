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
| 1 | **Needs Action** | `changes_requested` on a PR you authored, OR you're in `requested_reviewers` |
| 2 | **Waiting on Author** | PR by someone else, still open, and you reviewed it (`approved_by`/`changes_requested_by` = you) or commented on it (`viewer_commented`) |
| 3 | **Your PRs** | `pr.author === viewer` |
| 4 | **Your Code** | `touches_you` (existing Phase 4 field; lane renamed, field name unchanged) |
| 5 | **Mentioned** | `@your-login` appears in PR body or comments (new `mentioned` flag) |
| 6 | **FYI** | Everything else |

Ordering notes:

- "Your PRs" is checked before "Your Code" so your own PRs don't land in lane 4.
- "Waiting on Author" requires `author !== viewer`, so your own PRs never match it.
- When GitHub re-requests your review after you reviewed, `requested_reviewers` contains you again → the PR returns to lane 1.

### Lane assignment (renderer)

- New `electron/src/renderer/utils/lanes.ts`: pure `laneForPR(pr, viewerLogin) → 1..6`, exhaustively unit-tested. First-match-wins in the table order above.
- Inputs are all present in the PR list payload: `review_status`, `changes_requested_by`, `approved_by`, `requested_reviewers`, `author`, `touches_you`, plus two new fields below.
- Sort within a lane: `updated_at` DESC (existing default).
- `useVisiblePRs` remains the single canonical list source; lane grouping is a presentation layer over it, and `selectedPRIndex` continues to address the flat visible list.

### New Go-side signals

1. **`mentioned` flag** — migration 033 adds a `mentioned` column to `prs`. Computed at classify time by scanning the locally-synced PR body + comments for `@<viewer-login>` (no new GitHub API calls). Shipped in the PR list payload.
2. **`viewer_commented` flag** — computed field in the list payload (EXISTS subquery against the local `comments` table for the viewer's login). No migration. Caveat (accepted): list-only/unhydrated PRs have no comments synced, so the flag is false for them until hydration; such PRs sit in FYI.
3. **Read state** — migration 034 adds `pr_read_state (pr_id, login, last_read_at)`. A PR is unread when it has no row or `updated_at > last_read_at`. New WS message `MARK_PRS_READ {pr_ids}`: Go upserts rows and pushes the updated list. Sent by `Shift-R` (all PRs in current lane) and automatically when a PR is opened. Unread rows render bold with a dot prefix; read rows dimmed.

Go owns all SQLite writes, per convention.

### Keyboard

All bindings are pure config in `keybindings.ts` (sequence bindings are already supported):

- `g 1` … `g 6` — jump to first row of lane N.
- `j`/`k` — unchanged; walk the flat visible list across lane boundaries.
- `Shift-R` — mark all PRs in the current lane read.

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
| Side-by-side toggle | `s` toggles `diffStyle: 'unified' \| 'split'` (library prop). Persisted in settings as the default view mode. |
| Word-level intra-line diff | `lineDiffType: 'word'` + library's `maxLineDiffLength` cap. On by default; no UI. |
| Whitespace-only collapse | Custom: pure `isWhitespaceOnlyChange(hunk)` over parsed patch data (compare `-`/`+` lines after whitespace normalization). `w` toggles hiding those hunks behind a one-line "N whitespace-only lines hidden" marker. The library has no ignore-whitespace option (verified against 1.2.11 typings). |
| Generated-file collapse | Custom detection: constant pattern list (`*.lock`, `pnpm-lock.yaml`, `go.sum`, `*_generated.*`, `dist/`, linguist-generated conventions), not user-configurable. Collapse rendering uses the library's `collapsed` option (header stays visible). Manual expand/collapse per file. |
| Image/binary diffs | Deferred. Binary files render a placeholder row: "binary file — `o` opens in browser". |

---

## 5.3 Command palette polish

- **`:open in browser`** — palette command wired to the existing `list.openBrowser` action path (`o` keybinding + `openExternal` IPC already exist; wiring only).
- **`:open in editor`** — opens the current diff file at the current line:
  1. Look up the repo's local path (`repo_location`, migration 026). No local path → error toast.
  2. Verify the PR head SHA exists locally (`git cat-file -e <sha>` via a small Go WS request).
  3. SHA present → open `$EDITOR` at `file:line` using the `+line` CLI convention (`code -g file:line`, `vim +line`, fallback `$EDITOR file`), launched from the Electron main process.
  4. SHA missing → dialog: **"Branch not checked out"** with three choices: *Check out PR head* (Go runs `git fetch` + `checkout`, only if the worktree is clean — dirty tree returns an error toast), *Open current version* (open the local file as-is; may differ from the PR), *Cancel*.

---

## 5.4 First-run tour

- 5-screen overlay, new `OverlayKind: 'tour'`: (1) Inbox & lanes, (2) `j`/`k`/`enter` navigation, (3) diff review keys, (4) comment/approve actions, (5) command palette & `?` help.
- Key hints render from the actual `keybindings.ts` config so user overrides display correctly — no hardcoded key names.
- Shows once (settings flag), skippable with `Esc`, re-runnable via `:tour`.
- "Did you know?" tooltips: won't-do; ROADMAP updated accordingly.

---

## Error handling

- `MARK_PRS_READ` failures: existing WS action-queue semantics (queued offline, retried); UI shows the optimistic state and reconciles on the next list push.
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
