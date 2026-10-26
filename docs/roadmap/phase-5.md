# Phase 5 — Inbox & Triage Polish

[Roadmap](../ROADMAP.md)

## Phase 5 — Inbox & Triage Polish — **status: done (2026-08-09)**

By this point Bistro is feature-rich. Phase 5 makes it *fast to live in*.

> **Shipped early, off-roadmap (2026-06-24/25, verified 2026-07-11):**
> - **Markdown rendering** of PR descriptions and comments — sanitized
>   ReactMarkdown/GFM (`Markdown.tsx`, `markdownComponents.tsx`) incl. fenced-code
>   highlighting and **Mermaid diagrams** with strict security + size/concurrency
>   bounds (`MermaidDiagram.tsx`).
> - **Bistro design-system primitives** — restyled shadcn-style components under
>   `components/ui/` plus an integrated draggable title bar (`AppTitleBar.tsx`).

> **Implemented 2026-07-18 through 2026-08-09.** Authoritative docs (the
> original plan/spec were removed once the work shipped):
> - Lane rules and UI spec: `docs/DESIGN.md` (lane classifier lives in
>   `electron/src/renderer/utils/lanes.ts`)
> - CodeView migration spike notes: `docs/superpowers/plans/2026-07-18-codeview-spike-notes.md`
> - Shortcut reference: `docs/keyboard-shortcuts.md`
>
> **As-built deviations from the original draft below** (draft left intact for
> history; the spec is authoritative):
> - **Six-lane taxonomy**, not the four-lane sketch this section originally
>   described: Needs Action, Waiting on Author, Your PRs, Your Code, Mentioned,
>   FYI. See `docs/DESIGN.md` "Default lanes" for the shipped rules.
> - **Diff surface migrated to `@pierre/diffs` `CodeView`** (replacing the
>   hand-rolled TanStack-Virtual-around-`PatchDiff` component) before the
>   polish features landed — most of 5.2 became configuration afterward.
> - **Navigation remapping (post-implementation correction):** the diff route's
>   `j`/`k` now always scroll; `Shift+J`/`Shift+K` navigate files;
>   `n`/`p` navigate inline comments. PR-detail mode keeps `n`/`p` for file
>   navigation (it has no inline-comment view). This replaced an earlier
>   mapping where `j`/`k` tried comment navigation first and silently broke
>   scrolling on any file with threads.
> - **Image/binary diff rendering deferred** — a placeholder row
>   (`binary file — o opens in browser`) ships instead; full image diff is a
>   follow-up.
> - **React component tests excluded by user directive (2026-07-18).** Only
>   pure utilities, hooks, and non-trivial behavior are unit-tested; component
>   surfaces (lane headers, tour screens, dialogs) are verified manually.

### 5.1 Reviewer Inbox — done

**Acceptance.**

- Six-lane taxonomy: Needs Action, Waiting on Author, Your PRs, Your Code, Mentioned, FYI (see `docs/DESIGN.md`).
- Single keystroke moves between lanes (`g 1` … `g 6`).
- Per-lane keyboard navigation (`j/k`), continuous across lane boundaries in the flat visible list.
- Lane counts shown as section headers at all times.
- "Mark all read in lane" with `Shift-R`; PR read state persisted in SQLite (migration 034), Go engine owns the write.
- Auto-read on PR open (Enter, click, or palette — one code path).
- Repo-grouped view remains available via `:view groups` / Settings.

### 5.2 Diff polish — done (image diff deferred)

| Feature | Status | Notes |
|---|---|---|
| CodeView migration | done | Replaces hand-rolled TanStack Virtual; see spike notes doc |
| Side-by-side diff toggle | done | `Shift+S`; persisted in settings |
| Word-level diff within changed lines | done | `lineDiffType: 'word'`, on by default |
| Whitespace-only-change collapse | done | `w`; custom patch-rewriting (library has no ignore-whitespace option) |
| Generated-file collapse | done | Auto-collapse + `z` manual toggle per file |
| Viewed-file sidebar feedback | done | Sidebar icon switches `description` → `check_circle` when a file is marked viewed |
| Image/binary diff handling | deferred | Binary files render a placeholder row with a browser escape hatch (`o`); full image diff is a follow-up |

### 5.3 Command palette polish — done

| Command | Status | Notes |
|---|---|---|
| `:sync` (force sync now) | done (2026-06-12) | Sends FORCE_SYNC; footer shows SYNCING until SYNC_COMPLETE |
| `:settings`, `:about`, `:help` | done | Also implemented: `:clear` (clear filters/search), `:reset-store` |
| `:focus list/detail` | done | |
| `:view lanes` / `:view groups` | done | Switches `prList.view` |
| `:open in browser` | done | Palette command wired to the existing `list.openBrowser` action |
| `:diff split` / `:diff unified` | done | |
| `:open in editor` | done | Checks out the PR head on mismatch (with a confirmation dialog) before opening in `$EDITOR` |
| `:tour` | done | Brings back the first-run rail on the inbox |
| Command palette reachable from the diff route | done | `global.command` activation now includes `diff` (was list/detail only) |

### 5.4 Help / discoverability — done (tooltips won't-do)

| Item | Status | Notes |
|---|---|---|
| `?` overlay with all bindings | done | `HelpOverlay.tsx`; diff-review shortcuts shown in their own section |
| First-run tour | done | Non-modal first-review rail (`FirstRunRail.tsx`) after onboarding, re-runnable via `:tour`; replaced the 5-screen modal |
| "Did you know?" tooltips | won't-do | Deferred indefinitely — no plan to revisit |

---

