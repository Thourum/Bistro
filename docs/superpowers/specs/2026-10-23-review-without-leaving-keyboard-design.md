# Review Without Leaving the Keyboard — Design

**Date:** 2026-10-23
**Status:** Approved (user-reviewed design conversation)
**Roadmap:** Review-features priority #3 (mouse friction: 22 tags)
**Depends on:** `2026-10-23-since-last-review-design.md` §1 (merge-base diffs),
for correct left-side line numbers.

## Summary

Make a complete review possible from the keyboard: new **line comments**
(single and multi-line) collected as **pending drafts** and submitted as one
GitHub review; review actions in the **command palette**; a resolve toggle on
the active thread; and a **key-remapping UI** (phase 2, can ship separately).

## Current state (2026-10-23)

- Replies work; new line comments don't: `postComment` errors when `line > 0`
  (`go/internal/provider/github/reviews.go`), and the action queue refuses them.
- Action commands: `approve`, `request_changes`, `post_comment`,
  `reply_comment`, `resolve_thread`, `unresolve_thread`
  (`go/internal/actions/types.go`).
- `CommandPalette.tsx` has no review actions.
- Resolve/unresolve have keys only in the threads panel (`r`/`u`, `THREADS`
  scope).
- `pages/settings/KeyboardSection.tsx` says rebinding means editing
  `settings.json`.
- An open overlay takes precedence over route bindings
  (`utils/keybindingActivation.ts`), so a modal comment mode fits as an
  overlay kind.

## Phase 1 — Line comments, palette, resolve

### 1. Drafts (Go owns writes)

- Migration: `review_drafts(id, pr_id, login, path, side, start_line, line,
  body, commit_sha, created_at)`. `side` is `LEFT`/`RIGHT`; `start_line` is
  null for single-line comments; `commit_sha` is the head when drafted.
- WS: `DRAFT_SAVE` (create/update), `DRAFT_DELETE`, `DRAFTS_LIST`, and a
  `DRAFTS_CHANGED` push. Renderer validates with zod in `types/wsSchemas.ts` /
  `types/index.ts`.

### 2. Submitting

- New `submit_review` action: **one** `CreateReview` call with `event`
  (`APPROVE` | `REQUEST_CHANGES` | `COMMENT`), optional body, `comments[]`
  from the drafts (`path`, `side`, `line`, `start_side`/`start_line` for
  ranges), and `commit_id` = the drafts' `commit_sha`.
- Success deletes the submitted drafts. Failure keeps them, and the error names
  the drafts GitHub rejected (e.g. a 422 on a stale line).
- **Existing approve (`a a`) and request-changes (`Shift+R`) absorb pending
  drafts**: with drafts present they go through `submit_review` with that
  event. No separate submit step needed.
- Submissions are **not queued** when offline or rate-limited: they fail with
  drafts intact (drafts are already persisted locally).
- Drafts whose `commit_sha` is no longer the PR head are flagged "outdated" in
  the UI. They are still submitted against their own `commit_sha`, which
  GitHub accepts.

### 3. Comment mode (renderer)

- `c` in the diff (free in `DIFF` scope) opens a **line-pick overlay**; the
  cursor starts on the first changed line of the top visible file.
- In the overlay: `j`/`k` move the cursor; `Shift+J`/`Shift+K` extend the range
  (overriding the diff's next/prev-file bindings while the overlay is open); a
  range stays within one file and one side, as GitHub requires. In split view
  `h`/`l` switch sides. `Enter` opens the composer, `Esc` exits.
- Highlighting uses @pierre/diffs `setSelectedLines` / `SelectedLineRange`
  (`enableLineSelection`).
- Composer: `TEXTAREA_SUBMIT_HOTKEY` saves the draft; `Esc` discards.
- Drafts render inline as annotations with a **DRAFT** badge and are included
  in `n`/`p`. On a draft, `Enter` edits and `d d` deletes.
- Draft count in the diff status bar ("3 drafts") and on the PR detail page.
- The cursor/range logic is a pure function (move, extend, clamp to file and
  side) so it can be unit-tested without React.

### 4. Palette commands

On detail and diff pages: approve, request changes, comment, **submit review
(N drafts)**, discard drafts, start line comment, dismiss nits and info, dismiss
bots, undo dismissal, resolve / unresolve thread. Each entry dispatches the
**same handler as its key** (`utils/keybindingActions.ts`, by `actionId`) and
shows the bound key from `utils/keybindings.ts`, so behavior lives in one
place.

### 5. Resolve toggle

`x` (free in `DETAIL` and `DIFF`) toggles resolve on the active inline thread,
via the existing `resolve_thread` / `unresolve_thread` actions. Threads-panel
`r`/`u` stay.

## Phase 2 — Remapping UI (can ship separately)

- Keyboard settings page: `Enter` on an action row captures a new key or chord;
  `Backspace` resets to the default.
- Conflict detection uses the same route and overlay scopes as `isActive` in
  `utils/keybindingActivation.ts`: a conflict only exists when two bindings can
  be active at once.
- Writes `settings.keybindings`; replaces the "edit settings.json" note.
- Bindings stay pure config (`keys → actionId`); registration stays in
  `hooks/useAppHotkeys.ts`. Update CLAUDE.md's keybinding convention if the
  override shape changes.

## Out of scope

- Offline queueing of review submissions.
- Suggested-change blocks and file-level (non-line) comments.

## Testing (no React component tests)

- Go: fake GitHub (`httptest`) asserting the `CreateReview` payload — `side`,
  `line`, `start_line`/`start_side`, `commit_id`, and event mapping.
- Go: drafts survive a failed submit; succeeded drafts are deleted.
- TS: unit tests for the range-cursor function (move, extend, one file/side,
  clamping) and for remap conflict detection.
