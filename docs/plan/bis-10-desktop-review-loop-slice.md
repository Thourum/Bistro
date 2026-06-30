# BIS-10 Desktop Review-Loop Exploration

## Current implementation state

- Desktop client lives under `electron/src/` and is already structured as an Electron Forge + Vite + React app. Scripts in `electron/package.json` cover `pnpm start`, `pnpm type-check`, `pnpm lint`, and `pnpm test`.
- Main/preload boundary is in `electron/src/main/`: `ipc.ts` maps UI actions to Go sync actions, while `preload.ts` limits renderer channels. This keeps local file/engine access behind Electron IPC instead of direct renderer access.
- Review-loop UI surfaces are in `electron/src/renderer/`:
  - `pages/PRListPage.tsx` owns the inbox list, visible PR ids, filter/search/sort state, and triage lane counts.
  - `pages/PRDetailPage.tsx` resolves PRs by stable route id, requests comments/diff stats/CI checks, shows recent comments/files/thread overlay, and exposes approve/comment affordances.
  - `pages/DiffPage.tsx` owns the full diff/range-diff route, file identity via `?file`, comments revision refresh, `s` since-last-review intent, and `v` mark-viewed intent.
  - `components/PierreDiffView.tsx` renders local unified diffs, virtualized file views, inline thread annotations, and inline thread reply callbacks.
  - `components/CommentInput.tsx`, `CommentThread.tsx`, `UnresolvedThreadsView.tsx`, and `ActionFeedback.tsx` cover comment entry, thread display, unresolved thread navigation, and action feedback.
- Keyboard infrastructure is centralized as intended:
  - `utils/keybindings.ts` defines default bindings and route/overlay activation, including list navigation, `Space` to diff view, `aa` approve confirm, `c` comment on detail, `s` since-last-review, `v` mark viewed, `j/k` inline diff annotation movement, `Enter` inline reply, `gu` unresolved threads, and help/settings/search bindings.
  - `hooks/useAppHotkeys.ts` is the generic TanStack hotkey registration bridge that suppresses most bindings while typing.
  - `components/HelpOverlay.tsx` and tests keep discoverability aligned with the binding config.
- Action dispatch already exists in `hooks/useActionDispatch.ts` for `approve`, `request_changes`, `post_comment`, and `reply_comment`; `request_changes` is therefore supported at the dispatch/IPC layer but not surfaced as a keyboard-first PR-detail flow.
- Tests already cover the relevant seams: `keybindings.test.ts`, `useAppHotkeys.test.ts`, `useActionDispatch.test.ts`, `PRListPage.test.tsx`, `pr-navigation.test.tsx`, `diff-route.test.tsx`, `PRDetailPage.test.tsx`, `CommentInput.test.tsx`, and `CommentThread.test.tsx`.

## Gap

The smallest product gap in the keyboard-first review loop is **request changes from the PR detail route**. The backend/action plumbing exists, but the reviewer cannot complete the canonical triad — approve, comment, request changes — from the keyboard in the visible detail workflow. This leaves the README promise partially fulfilled: approve/comment are surfaced, but request changes is only a lower-level dispatch capability.

## Recommended next implementation slice

**Slice: Keyboard-first request-changes composer on PR detail.**

Scope:

1. Add a default keybinding in `electron/src/renderer/utils/keybindings.ts` for `detail.requestChanges` on the PR detail route. Suggested key: `r c` or `Shift+R`; avoid conflicting with existing reply/thread bindings.
2. Extend `PRDetailPage.tsx` so the new binding sets `confirmingAction: 'request_changes'` and opens a focused composer using the existing `CommentInput`/composer pattern.
3. Wire submit to `dispatchRequestChanges(comment)` from `useActionDispatch(pr.id)`; keep the renderer as an IPC client only.
4. Show existing `ActionFeedback` success/failure/pending states and clear/cancel on `Escape` consistently with the current comment composer.
5. Update `HelpOverlay.tsx`/footer legend labels so request changes is discoverable beside approve/comment.
6. Add/extend unit tests for keybinding activation, hotkey behavior, PR detail composer submit/cancel, and `useActionDispatch` payload expectations.

Non-goals:

- Do not change GitHub API policy, retry/backoff semantics, or Go action-queue behavior.
- Do not add new SQLite writes from Electron.
- Do not redesign the diff viewer or comment thread model.
- Do not add AI classification or marketing copy.

Risks:

- Key collision with existing detail/diff/thread navigation if the chord is not route-scoped carefully.
- Confusing composer modes if `comment`, `reply`, and `request_changes` reuse the same `confirmingAction` field without explicit labels/placeholders.
- Offline action confidence depends on existing Go queue semantics; this slice should only reuse the established IPC path.

Verification:

- `cd electron && pnpm test -- tests/unit/keybindings.test.ts tests/unit/useAppHotkeys.test.ts tests/unit/useActionDispatch.test.ts tests/unit/PRDetailPage.test.tsx`
- `cd electron && pnpm type-check`

## Follow-up issue recommendation

Create one child issue for the slice above. It is small enough for one implementation task and should not be split unless the chosen keybinding/UX requires design review.
