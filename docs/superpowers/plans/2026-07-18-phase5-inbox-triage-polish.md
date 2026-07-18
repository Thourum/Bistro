# Phase 5 — Inbox & Triage Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement ROADMAP Phase 5 per the approved spec at `docs/superpowers/specs/2026-07-18-phase5-inbox-triage-polish-design.md`: a lane-grouped Reviewer Inbox with read state, diff polish on a CodeView foundation, two palette commands, and a first-run tour.

**Architecture:** Go sync engine owns all SQLite writes and ships raw signals (`mentioned`, `viewer_commented`, `unread`, `viewer_login`) in the PR list payload; the renderer computes lane assignment with a pure function and renders lanes as section headers over the existing flat list. The diff surface migrates from hand-rolled TanStack-Virtual-around-`PatchDiff` to the library's `CodeView`, after which most diff polish is configuration.

**Tech Stack:** Go 1.26 (goose migrations, gorilla/websocket), Electron + React 19 + TypeScript, `@pierre/diffs` 1.2.11 (`CodeView`), TanStack Store/Hotkeys, Vitest, Biome. Errors via `neverthrow` in TS — **never `try`/`catch`**; Go returns errors.

**Conventions the implementer must know (from CLAUDE.md):**
- Go owns ALL SQLite writes. The renderer never writes the DB.
- TS error handling: `neverthrow` `Result`/`ResultAsync` only.
- Keybindings are pure config in `electron/src/renderer/utils/keybindings.ts`; registration lives in `hooks/useAppHotkeys.ts`.
- Verify Go: `cd go && go vet ./... && go test -race ./...`. Verify Electron: `cd electron && pnpm type-check && pnpm lint && pnpm test`.
- Commit format: `<type>: <description>` (feat/fix/refactor/docs/test/chore).

**Existing patterns to copy:**
- Migration file: `go/internal/store/migrations/00032_add_pr_hydrated.sql` (goose `-- +goose Up` format; migrations are embedded and auto-run).
- Migration schema test: `TestMigration014AddsLastReviewedAndFilesViewed` in `go/internal/store/db_test.go`.
- WS handler: `NewMarkFileViewedHandler` in `go/internal/server/handler/patchsets.go`, registered in `go/internal/server/app.go` (~line 647) via `a.wsServer.RegisterHandler("MARK_FILE_VIEWED", ...)`.
- Renderer WS mutation hook: `electron/src/renderer/hooks/useMarkFileViewed.ts`.
- PR list SQL + payload: `REQUEST_PR_LIST` handler in `go/internal/server/handler/pr.go` (SELECT at ~line 103, `prRow` struct with `json` tags at top, `touches_you` resolution ~line 159).

---

## Task order & dependencies

Tasks 1–7 (Inbox) are independent of Tasks 8–14 (Diff) and 15–17 (Palette) and 18 (Tour). Within each group, do tasks in order. Task 8 (spike) gates tasks 9–14.

---

### Task 1: Migration 033 — `mentioned` column on `prs`

**Files:**
- Create: `go/internal/store/migrations/00033_add_pr_mentioned.sql`
- Test: `go/internal/store/db_test.go` (append)

- [ ] **Step 1: Write the failing schema test** in `db_test.go`, copying the shape of `TestMigration014AddsLastReviewedAndFilesViewed`:

```go
func TestMigration033AddsPRMentioned(t *testing.T) {
	db := openTestDB(t) // use the same helper the sibling migration tests use
	cols := tableColumns(t, db, "prs")
	if !cols["mentioned"] {
		t.Error("prs missing column mentioned")
	}
}
```

- [ ] **Step 2: Run it to make sure it fails**

Run: `cd go && go test ./internal/store/ -run TestMigration033 -v`
Expected: FAIL (column missing)

- [ ] **Step 3: Write the migration**

```sql
-- +goose Up
-- Migration: 00033_add_pr_mentioned
-- Description: Phase 5 inbox — whether the current viewer's @login appears in
-- the PR body or any synced comment. Computed locally at classify time from
-- already-synced data; no extra GitHub API calls.

ALTER TABLE prs ADD COLUMN mentioned INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- This migration is irreversible.
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd go && go test ./internal/store/ -run TestMigration033 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go/internal/store/migrations/00033_add_pr_mentioned.sql go/internal/store/db_test.go
git commit -m "feat(store): add prs.mentioned column (migration 033)"
```

---

### Task 2: Compute `mentioned` during sync

The sync pipeline already post-processes PRs after comments land (the Phase 4 `touches_you` post-diff hook in `go/internal/sync/` is the precedent — find it with `grep -rn "touches_you\|post-diff" go/internal/sync/`). Add a step that sets `mentioned=1` when `@<current-login>` appears (case-insensitive, word-boundary) in `prs.body` or any row of `comments` for that PR.

**Files:**
- Modify: the sync post-processing site found above (same package)
- Test: sibling `_test.go` in that package

- [ ] **Step 1: Write failing test.** Seed an in-memory store with a PR whose body contains `hey @alice please look` and a current user `alice`; run the hook; assert `mentioned=1`. Add cases: mention only in a comment; `@alice2` must NOT match `alice` (word boundary); case-insensitive `@Alice` matches.

- [ ] **Step 2: Run it, confirm FAIL** (`go test ./internal/sync/ -run Mentioned -v`).

- [ ] **Step 3: Implement.** Regex `(?i)@alice\b` built with `regexp.QuoteMeta(login)`. One UPDATE per PR batch:

```go
// UPDATE prs SET mentioned = ? WHERE id = ?
```

Match against `pr.Body` and a single `SELECT body FROM comments WHERE pr_id = ?` pass. Keep it in the same transaction the hook already uses.

- [ ] **Step 4: Run tests, confirm PASS.** Also run the full package: `go test -race ./internal/sync/`.

- [ ] **Step 5: Commit** — `feat(sync): compute prs.mentioned from body and comments`

---

### Task 3: Migration 034 — `pr_read_state` + `MARK_PRS_READ` handler

**Files:**
- Create: `go/internal/store/migrations/00034_add_pr_read_state.sql`
- Create: `go/internal/server/handler/readstate.go`
- Create: `go/internal/server/handler/readstate_test.go`
- Modify: `go/internal/server/app.go` (register handler, ~line 647 block)
- Test: `go/internal/store/db_test.go` (schema test)

- [ ] **Step 1: Failing schema test** (`TestMigration034AddsPRReadState`): table `pr_read_state` with columns `pr_id`, `login`, `last_read_at`; upsert on `(pr_id, login)` replaces `last_read_at`.

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Migration:**

```sql
-- +goose Up
-- Migration: 00034_add_pr_read_state
-- Description: Phase 5 inbox read state. A PR is unread for a login when no
-- row exists or prs.updated_at > last_read_at.

CREATE TABLE IF NOT EXISTS pr_read_state (
    pr_id        TEXT NOT NULL,
    login        TEXT NOT NULL,
    last_read_at TIMESTAMP NOT NULL,
    PRIMARY KEY (pr_id, login)
);

-- +goose Down
-- This migration is irreversible.
```

- [ ] **Step 4: Run schema test, confirm PASS.**

- [ ] **Step 5: Failing handler test** in `readstate_test.go`, copying the harness from an existing handler test (`comments_test.go` or `patchsets_test.go`): send `MARK_PRS_READ` with `{"pr_ids": ["p1","p2"]}`, assert ack `{ok: true, marked: 2}` and rows exist with fresh `last_read_at`. Error cases: empty `pr_ids` → error; no current user → `ok:false` skip (copy the `MARK_FILE_VIEWED` "no current user" behavior).

- [ ] **Step 6: Run, confirm FAIL.**

- [ ] **Step 7: Implement `NewMarkPRsReadHandler`** modeled on `NewMarkFileViewedHandler`:
  - Parse `{pr_ids []string}`; validate non-empty, cap at 500 ids.
  - Resolve current login the same way `MARK_FILE_VIEWED` does.
  - `INSERT INTO pr_read_state (pr_id, login, last_read_at) VALUES (?, ?, CURRENT_TIMESTAMP) ON CONFLICT(pr_id, login) DO UPDATE SET last_read_at = excluded.last_read_at` in one transaction.
  - Return `{ok: true, marked: N}`.
  - Register in `app.go`: `a.wsServer.RegisterHandler("MARK_PRS_READ", handler.NewMarkPRsReadHandler(...))` next to `MARK_FILE_VIEWED`.

- [ ] **Step 8: Run handler tests + `go vet ./...`, confirm PASS.**

- [ ] **Step 9: Commit** — `feat(server): pr_read_state table and MARK_PRS_READ handler`

---

### Task 4: Extend `REQUEST_PR_LIST` payload — `mentioned`, `viewer_commented`, `unread`, `viewer_login`

**Files:**
- Modify: `go/internal/server/handler/pr.go`
- Test: `go/internal/server/handler/pr_test.go` (or create; follow sibling handler tests)

- [ ] **Step 1: Failing test.** Seed PRs + comments + read state; request `REQUEST_PR_LIST`; assert each PR row has `mentioned`, `viewer_commented`, `unread` booleans and the payload `data` carries `viewer_login`. Cases: PR with viewer comment → `viewer_commented: true`; PR with `updated_at` newer than `last_read_at` → `unread: true`; no read-state row → `unread: true`; marked read after update → `unread: false`.

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Implement.**
  - Add to `prRow`: `Mentioned bool \`json:"mentioned"\``, `ViewerCommented bool \`json:"viewer_commented"\``, `Unread bool \`json:"unread"\``.
  - Extend the SELECT (login resolved once, passed as a bind param; keep parameterized — no string concat of the login):

```sql
SELECT ..., mentioned,
  EXISTS(SELECT 1 FROM comments c WHERE c.pr_id = prs.id AND c.author = ?) AS viewer_commented,
  (COALESCE((SELECT r.last_read_at FROM pr_read_state r
             WHERE r.pr_id = prs.id AND r.login = ?), '1970-01-01') < prs.updated_at) AS unread
FROM prs WHERE 1=1 ...
```

  - Add `"viewer_login": login` to the `PR_LIST_READY` `data` map.
  - When no current user is available, ship `mentioned` from the column, `viewer_commented:false`, `unread:false`, `viewer_login:""` (inbox degrades to FYI-heavy — acceptable pre-auth).

- [ ] **Step 4: Run `go test -race ./internal/server/...` + `go vet ./...`, confirm PASS.**

- [ ] **Step 5: Commit** — `feat(server): inbox signals in PR list payload`

---

### Task 5: Renderer lane model — `lanes.ts` + types

**Files:**
- Modify: `electron/src/renderer/types/index.ts` (PR interface: add `mentioned: boolean; viewer_commented: boolean; unread: boolean;`)
- Modify: the `PR_LIST_READY` consumer (`electron/src/renderer/hooks/useWebSocketPRList.ts`) to surface `viewer_login` (store it in the app store as `viewerLogin: string`)
- Create: `electron/src/renderer/utils/lanes.ts`
- Create: `electron/src/renderer/utils/lanes.test.ts`

- [ ] **Step 1: Write the failing tests first** — this is the core logic of the feature; be exhaustive:

```typescript
import { describe, expect, test } from 'vitest';
import { LANES, laneForPR } from './lanes';

const base = {
  author: 'bob', state: 'open', review_status: null,
  approved_by: null, changes_requested_by: null,
  requested_reviewers: [] as string[],
  touches_you: false, mentioned: false, viewer_commented: false,
} as never; // build a PR factory matching the real PR type in the actual file

describe('laneForPR', () => {
  test('changes requested on your PR → lane 1', () =>
    expect(laneForPR({ ...base, author: 'me', review_status: 'changes_requested' }, 'me')).toBe(1));
  test('you are a requested reviewer → lane 1 (no age split)', () =>
    expect(laneForPR({ ...base, requested_reviewers: ['me'] }, 'me')).toBe(1));
  test('you reviewed someone else's open PR → lane 2', () =>
    expect(laneForPR({ ...base, changes_requested_by: 'me' }, 'me')).toBe(2));
  test('you commented on someone else's open PR → lane 2', () =>
    expect(laneForPR({ ...base, viewer_commented: true }, 'me')).toBe(2));
  test('re-requested review wins over waiting-on-author → lane 1', () =>
    expect(laneForPR({ ...base, changes_requested_by: 'me', requested_reviewers: ['me'] }, 'me')).toBe(1));
  test('your own PR never lands in waiting-on-author → lane 3', () =>
    expect(laneForPR({ ...base, author: 'me', viewer_commented: true }, 'me')).toBe(3));
  test('your PR checked before your code → lane 3 even when touches_you', () =>
    expect(laneForPR({ ...base, author: 'me', touches_you: true }, 'me')).toBe(3));
  test('touches your code → lane 4', () =>
    expect(laneForPR({ ...base, touches_you: true }, 'me')).toBe(4));
  test('mentioned → lane 5', () =>
    expect(laneForPR({ ...base, mentioned: true }, 'me')).toBe(5));
  test('everything else → lane 6', () =>
    expect(laneForPR(base, 'me')).toBe(6));
  test('empty viewer login → everything is FYI except touches/mentioned', () =>
    expect(laneForPR({ ...base, requested_reviewers: ['me'] }, '')).toBe(6));
});
```

- [ ] **Step 2: Run** `cd electron && pnpm test lanes` — FAIL (module missing).

- [ ] **Step 3: Implement `lanes.ts`** — pure, no store imports:

```typescript
import type { PR } from '../types';

export const LANES = [
  { id: 1, key: 'needs_action', label: 'NEEDS ACTION' },
  { id: 2, key: 'waiting_on_author', label: 'WAITING ON AUTHOR' },
  { id: 3, key: 'your_prs', label: 'YOUR PRS' },
  { id: 4, key: 'your_code', label: 'YOUR CODE' },
  { id: 5, key: 'mentioned', label: 'MENTIONED' },
  { id: 6, key: 'fyi', label: 'FYI' },
] as const;

export type LaneId = (typeof LANES)[number]['id'];

/** First-match-wins lane assignment. Spec: 2026-07-18 Phase 5 design §5.1. */
export function laneForPR(pr: PR, viewerLogin: string): LaneId {
  const isViewer = viewerLogin !== '';
  const isAuthor = isViewer && pr.author === viewerLogin;
  if (isViewer) {
    if (isAuthor && pr.review_status === 'changes_requested') return 1;
    if (pr.requested_reviewers.includes(viewerLogin)) return 1;
    const reviewed =
      pr.approved_by === viewerLogin || pr.changes_requested_by === viewerLogin;
    if (!isAuthor && pr.state === 'open' && (reviewed || pr.viewer_commented)) return 2;
    if (isAuthor) return 3;
  }
  if (pr.touches_you) return 4;
  if (pr.mentioned) return 5;
  return 6;
}
```

- [ ] **Step 4: Run tests, confirm PASS.** Also `pnpm type-check`.

- [ ] **Step 5: Wire payload fields**: add the three booleans to the `PR` interface; in `useWebSocketPRList.ts`, read `viewer_login` from `PR_LIST_READY` data and write it to the app store (`viewerLogin`, default `''`, in `store/ui.ts`).

- [ ] **Step 6: `pnpm type-check && pnpm lint && pnpm test`, all green. Commit** — `feat(inbox): lane assignment model`

---

### Task 6: Inbox UI — lane sections, unread styling, ordering

**Files:**
- Modify: `electron/src/renderer/hooks/useVisiblePRs.ts` — when the list view is `flat` (inbox mode), order the canonical list by `(lane, updated_at DESC)` so `selectedPRIndex`/`j`/`k` naturally walk lanes in order. Lane of each PR comes from `laneForPR(pr, viewerLogin)`.
- Modify: `electron/src/renderer/components/PRListTable.tsx` — render a lane header row before the first PR of each lane (list is already lane-sorted): `<laneId> <LABEL> — <count>` in the existing uppercase/mono style (reuse the repo-group `<summary>` styling); render ALL six headers including empty lanes (empty: header + dimmed `— clear —` row). Unread rows: title `font-semibold` + `●` prefix; read rows: existing dimmed styles.
- Test: `electron/src/renderer/components/PRListTable.test.tsx` (extend existing tests)

- [ ] **Step 1: Failing component test:** given PRs mapped to lanes 1/4/6, the table renders six headers with counts `1 NEEDS ACTION — 1`, `4 YOUR CODE — 1`, `6 FYI — 1`, empty lanes show `— clear —`, and an unread PR row shows the `●` marker.

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Implement** (keep `useVisiblePRs` the single source of truth; lane grouping is presentation over the flat sorted list — compute `laneForPR` once per PR in a `useMemo`, don't re-derive per row).

- [ ] **Step 4: Tests + type-check + lint green.**

- [ ] **Step 5: Manual check:** `cd electron && pnpm start`, verify lanes render and repo-grouped view (settings `prList.view`) still works.

- [ ] **Step 6: Commit** — `feat(inbox): lane-sectioned PR list with unread markers`

---

### Task 7: Inbox keyboard — `g 1`…`g 6`, `Shift-R`, auto-read on open

**Files:**
- Modify: `electron/src/renderer/utils/keybindings.ts` (bindings; sequences like `['g','1']` are supported — see `list.top: ['g','g']`)
- Create: `electron/src/renderer/hooks/useMarkPRsRead.ts` (copy `useMarkFileViewed.ts` pattern; message `MARK_PRS_READ`, payload `{pr_ids}`; on ack, refetch/invalidate the PR list the same way other mutations do)
- Modify: `electron/src/renderer/components/PRListTable.tsx` or the open-navigation site — send `MARK_PRS_READ` for a PR when it is opened
- Test: `electron/src/renderer/utils/keybindings.test.ts` if binding-level tests exist (check first); otherwise cover via `lanes.test.ts` helpers + hook test

- [ ] **Step 1:** Add bindings config (test-first where a test harness exists):
  - `inbox.lane1`…`inbox.lane6`: keys `['g','1']`…`['g','6']`, route `list`, hidden: lanes 2–6 (avoid help-overlay noise, same convention as jump keys), description "Jump to lane N". Action: set `selectedPRIndex` to the first PR of lane N in the canonical list (no-op if lane empty).
  - `inbox.markLaneRead`: keys `'Shift+R'`, route `list`. Action: collect filter-visible PRs in the selected row's lane; call `markPRsRead(ids)`; no-op when the list is empty. NOTE: `Shift+R` is `detail.requestChanges` on the `detail` route — route gating keeps them disjoint; verify with the help overlay.
- [ ] **Step 2:** Implement `useMarkPRsRead` + auto-mark-on-open (fire-and-forget on the open action, `.catch`-free — neverthrow style per the existing hook).
- [ ] **Step 3:** `pnpm type-check && pnpm lint && pnpm test` green. Manual: `g 3` jumps, `Shift-R` clears bold, opening a PR clears its dot.
- [ ] **Step 4: Commit** — `feat(inbox): lane jump keys, Shift-R mark-read, auto-read on open`

---

### Task 8: CodeView verification spike (GATE for tasks 9–14)

**Files:** throwaway branch/scratch — nothing committed except a findings note.

- [ ] **Step 1:** Read the CodeView docs section: https://diffs.com/docs#codeview (and `dist/*.d.ts` typings in `node_modules/@pierre/diffs`).
- [ ] **Step 2:** In a scratch copy of the diff route, render a real synced PR (large one preferred) through `CodeView` with: unified+split toggle, `DiffLineAnnotation` comment annotations (the current `PierreDiffView` annotation mapping, see its `thread → annotation` logic ~line 126/536), and keyboard scrolling (`j/k/n/p` equivalents via CodeView's scroll targets — `CodeViewLineScrollTarget`/`CodeViewItemScrollTarget` exist in typings).
- [ ] **Step 3:** Compare `pnpm bench:large-diff` (baseline exists in `electron/scripts/bench-large-diff.mjs`) — record before/after numbers.
- [ ] **Step 4:** Decision: annotations + keyboard nav + perf all workable → proceed to Task 9. Any hard failure → STOP, fall back to the spec's PatchDiff fallback (side-by-side/word-diff as props on existing view; re-plan tasks 9–14 minimally) and tell the human.
- [ ] **Step 5:** Write findings to `docs/superpowers/plans/2026-07-18-codeview-spike-notes.md`; commit — `docs: CodeView spike findings`

---

### Task 9: Migrate diff surface to CodeView

**Files:**
- Modify: `electron/src/renderer/components/PierreDiffView.tsx` → split into `components/diff/` modules: `DiffView.tsx` (CodeView host, keeps public props), `diffAnnotations.ts` (thread→annotation mapping), `diffFileSidebar.tsx` (existing `<aside>` file list), `diffPathUtils.ts` (the path/rename helpers at ~line 961–1056, pure — move verbatim with their behavior locked by tests)
- Delete: TanStack Virtual scaffolding inside the old component (the `useVirtualizer` usage)
- Test: unit tests for `diffPathUtils.ts` (write BEFORE moving: capture current behavior for renames/deletions/`/dev/null` cases); existing diff-related tests keep passing

- [ ] **Step 1:** Write `diffPathUtils.test.ts` against the CURRENT functions (import from `PierreDiffView.tsx` temporarily) — rename returns NEW path, deletion falls back to a/ side, same-path headers. Run: PASS (locks behavior).
- [ ] **Step 2:** Extract the pure modules; re-run tests: PASS.
- [ ] **Step 3:** Swap `PatchDiff`+virtualizer for `CodeView` per spike findings. Keep: comment threads, inline comment composer, `DegradedRangeDiffIndicator`, expertise `formatWhy` annotations, mark-viewed (`v`) flow, since-last-review (`s`) range diff entry point.
- [ ] **Step 4:** `pnpm type-check && pnpm lint && pnpm test` green; `pnpm bench:large-diff` not worse than baseline; manual review of a real PR (threads visible, `n/p/j/k` work).
- [ ] **Step 5: Commit** — `refactor(diff): migrate to @pierre/diffs CodeView`

---

### Task 10: Side-by-side toggle (`Shift+S`)

**Files:**
- Modify: `electron/src/types/settings.ts` — add to a new `diff` settings section: `{ diff: { style: 'unified' | 'split' } }`, default `'unified'`, extend `DEFAULT_SETTINGS`
- Modify: `electron/src/renderer/utils/keybindings.ts` — `diff.toggleStyle`, keys `'Shift+S'`, route `diff` (plain `s` stays `diff.sinceLastReview`)
- Modify: `components/diff/DiffView.tsx` — pass `diffStyle` from settings
- Test: settings default test if a settings schema test exists; binding smoke via existing patterns

- [ ] **Step 1:** Add setting + default; type-check.
- [ ] **Step 2:** Add binding that flips the setting (persisted via the existing settings write path — find with `grep -rn 'updateSettings\|setSettings' electron/src/renderer | head`).
- [ ] **Step 3:** Pass `diffStyle` into CodeView options.
- [ ] **Step 4:** Manual: `Shift+S` flips unified↔split live; restart preserves choice. All checks green.
- [ ] **Step 5: Commit** — `feat(diff): side-by-side toggle on Shift+S`

---

### Task 11: Word-level intra-line diff

**Files:**
- Modify: `components/diff/DiffView.tsx` — CodeView render options: `lineDiffType: 'word'` (library default cap `maxLineDiffLength` kept). If the worker pool is used (check whether the migration introduced `WorkerPoolContextProvider`), set it there via `setRenderOptions` instead — worker pool owns `lineDiffType`.

- [ ] **Step 1:** Set the option; verify visually on a PR with edited lines (changed spans darker).
- [ ] **Step 2:** `pnpm type-check && pnpm lint`; bench unchanged.
- [ ] **Step 3: Commit** — `feat(diff): word-level intra-line highlighting`

---

### Task 12: Whitespace-only-change collapse (`w`)

**Files:**
- Create: `electron/src/renderer/utils/whitespaceDiff.ts` + `whitespaceDiff.test.ts`
- Modify: `components/diff/DiffView.tsx` (filter/annotate hunks), `utils/keybindings.ts` (`diff.toggleWhitespace`, keys `'w'`, route `diff` — `w` is free on diff route)

- [ ] **Step 1: Failing tests** for the pure detector:

```typescript
// isWhitespaceOnlyHunk(hunk): every removed line, after collapsing all
// whitespace runs and trimming, equals its paired added line; hunks with
// adds/removes that change non-whitespace → false; pure-context hunks → false.
test('indentation-only change → true', ...)
test('tab→space conversion → true', ...)
test('real code change → false', ...)
test('line added (no pair) → false', ...)
```

Operate on the parsed patch structures the diff view already holds (`parsePatchFiles` output — `FileDiffMetadata.hunks`), normalization: `line.replace(/\s+/g, '')` comparison of the joined removed vs added sides.

- [ ] **Step 2:** Run — FAIL. Implement. Run — PASS.
- [ ] **Step 3:** Wire `w`: when hidden, replace those hunks with a rendered marker row "N whitespace-only lines hidden" (CodeView injected-row or a filtered item list — per spike findings). Toggle state is route-local (app store), not persisted.
- [ ] **Step 4:** All checks green; manual on a whitespace-churn PR. Commit — `feat(diff): whitespace-only hunk collapse on w`

---

### Task 13: Generated-file collapse

**Files:**
- Create: `electron/src/renderer/utils/generatedFiles.ts` + `generatedFiles.test.ts`
- Modify: `components/diff/DiffView.tsx` — pass `collapsed: true` (library option keeps header visible) for detected files; manual expand/collapse per file (library's built-in header interaction or a keybinding on the focused file — follow what CodeView exposes, per spike)

- [ ] **Step 1: Failing tests:**

```typescript
// isGeneratedFile(path): true for '*.lock', 'pnpm-lock.yaml', 'go.sum',
// 'package-lock.json', '*_generated.*', '*.pb.go', paths under 'dist/',
// '.min.js'; false for ordinary sources. Constant list, not configurable.
```

- [ ] **Step 2:** Run — FAIL. Implement (a `GENERATED_PATTERNS: RegExp[]` constant + `some()` — no config). Run — PASS.
- [ ] **Step 3:** Wire `collapsed` default + manual toggle. All checks green. Commit — `feat(diff): auto-collapse generated files`

---

### Task 14: Binary-file placeholder

**Files:**
- Modify: `components/diff/DiffView.tsx`
- Modify: whatever module builds the file's GitHub URL (reuse the `openExternal` IPC + PR URL builder used by `list.openBrowser` — find with `grep -rn 'openExternal' electron/src`)

- [ ] **Step 1:** Detect binary patches (unified diff `Binary files ... differ` marker / `FileDiffMetadata` binary flag if present in typings). Render placeholder row: `binary file — o opens in browser`.
- [ ] **Step 2:** `o` on the focused binary file opens `https://github.com/<owner>/<repo>/pull/<n>/files#diff-<sha256-of-path>` (GitHub file anchor = sha256 hex of the file path — verify format once in a browser before hardcoding; if anchor scheme is wrong, fall back to the plain `/files` page).
- [ ] **Step 3:** Checks green; manual with a PR containing an image. Commit — `feat(diff): binary file placeholder with browser escape hatch`

---

### Task 15: Palette `:open in browser`

**Files:**
- Modify: `electron/src/renderer/components/CommandPalette.tsx` (commands array, entries like `{id, name, icon}` at ~line 36) + its dispatch switch
- Test: `CommandPalette.test.tsx` (existing file — extend)

- [ ] **Step 1: Failing test:** palette lists "Open in browser" and selecting it invokes the same action as the `o` keybinding for the selected PR.
- [ ] **Step 2:** Implement by calling the existing `list.openBrowser` action path (do NOT duplicate URL building).
- [ ] **Step 3:** Tests green. Commit — `feat(palette): open in browser command`

---

### Task 16: Go — `CHECK_PR_CHECKOUT` and `CHECKOUT_PR_HEAD` handlers

**Files:**
- Create: `go/internal/server/handler/editor.go` + `editor_test.go`
- Modify: `go/internal/server/app.go` (register both)

Semantics (spec §5.3): `CHECK_PR_CHECKOUT {pr_id}` → `{local_path, head_matches, sha_present}` where `head_matches` = repo HEAD equals PR `head_sha` (`git rev-parse HEAD` via go-git), `sha_present` = object exists (`cat-file -e` equivalent). `CHECKOUT_PR_HEAD {pr_id}` → fetch (only when `!sha_present`) + checkout `head_sha`; refuse with a typed error when the worktree is dirty.

- [ ] **Step 1: Failing tests** (use a temp git repo fixture — the repo already has go-git test helpers, find with `grep -rn 'PlainInit\|fixture' go/internal/git/ go/internal/scan/ | head`):
  - HEAD at PR sha → `head_matches: true`.
  - HEAD elsewhere, object present → `{head_matches: false, sha_present: true}`.
  - Object absent → `sha_present: false`.
  - Dirty worktree + checkout → error `"worktree dirty"`, no state change.
  - Clean worktree + checkout → HEAD moves to sha.
  - PR repo has no `repo_location` → typed error `"no local path"`.
- [ ] **Step 2:** Run — FAIL.
- [ ] **Step 3:** Implement with go-git (`Repository.Head()`, `Object()` existence check, `Worktree().Status().IsClean()`, `Worktree().Checkout(&git.CheckoutOptions{Hash})`, `Fetch` with the existing auth transport used elsewhere in `go/internal/git/`). Look up local path from the `repo_location` data (migration 026 — find the accessor with `grep -rn 'repo_location' go/internal/store/`).
- [ ] **Step 4:** `go vet ./... && go test -race ./internal/server/...` green.
- [ ] **Step 5: Commit** — `feat(server): PR checkout inspection and checkout handlers`

---

### Task 17: Renderer `:open in editor`

**Files:**
- Modify: `electron/src/renderer/components/CommandPalette.tsx` (command entry, `diff`-route-aware)
- Create: `electron/src/renderer/hooks/useOpenInEditor.ts` (WS calls + dialog state)
- Create: `electron/src/main/` IPC: `open-in-editor` channel (grep `openExternal` registration in `electron/src/main/` and add a sibling) — spawns the editor from the main process
- Test: `useOpenInEditor.test.ts` for the decision logic (mock WS)

Current file/line model (spec): file = `n`/`p` focus target; line = selected inline-comment line, else first new-side line of the file's first hunk, else 1.

- [ ] **Step 1: Failing hook test:** given `{head_matches: true}` → resolves editor launch args; `{head_matches: false}` → surfaces dialog state with three options; "no local path" error → toast text. neverthrow `ResultAsync` chains, no try/catch.
- [ ] **Step 2:** Run — FAIL. Implement hook.
- [ ] **Step 3:** Main-process IPC: build args by editor family — `code`/`cursor`: `-g file:line`; `vim`/`nvim`/terminal editors are out (GUI context): fall back to `$EDITOR file` without line when the `+line` convention is unknown; `$EDITOR` unset → error toast "Set $EDITOR". Validate that the resolved path stays inside the repo local path (reject `..` traversal) before spawning; spawn with an args array (no shell string).
- [ ] **Step 4:** Dialog component: "PR revision not checked out" → [Check out PR head] (calls `CHECKOUT_PR_HEAD`, then opens), [Open current version] (opens with mismatch warning in dialog body), [Cancel]. Reuse the existing dialog primitives (`components/ui/`).
- [ ] **Step 5:** All checks green; manual end-to-end with a locally-checked-out repo. Commit — `feat(palette): open in editor with checkout verification`

---

### Task 18: First-run tour

**Files:**
- Modify: `electron/src/renderer/overlays/OverlayContext.tsx` — add `'tour'` to `OverlayKind`
- Create: `electron/src/renderer/overlays/TourOverlay.tsx` (5 screens as a const array; arrow/enter advances, Esc dismisses)
- Modify: `electron/src/types/settings.ts` — `appearance.tourComplete: boolean` default `false` (sibling of `onboardingComplete`)
- Modify: `electron/src/renderer/components/CommandPalette.tsx` — `:tour` command re-opens it
- Modify: app shell (where onboarding gating lives — `grep -rn 'onboardingComplete' electron/src/renderer`) — auto-open tour once after onboarding completes
- Test: `TourOverlay.test.tsx`

Screens (content, keys rendered from `keybindings.ts` config via the same lookup the help overlay uses — no hardcoded key names; check `HelpOverlay.tsx` for the accessor):
1. Inbox & lanes (`g 1`–`g 6`, `Shift-R`)
2. Navigation (`j`/`k`/`Enter`, `/` search, `f` filter)
3. Diff review (`n`/`p`, `Shift+S` split, `w` whitespace, `v` viewed, `s` since-last-review)
4. Actions (`c` comment, `a a` approve, `Shift+R` request changes, `d n`/`d b` dismiss)
5. Palette & help (`:` commands, `?` bindings)

- [ ] **Step 1: Failing tests:** renders screen 1 of 5; advances; Esc closes and persists `tourComplete: true`; shows override key when a binding is overridden.
- [ ] **Step 2:** Run — FAIL. Implement.
- [ ] **Step 3:** Wire show-once + `:tour`. All checks green. Commit — `feat(help): 5-screen first-run tour`

---

### Task 19: Docs + roadmap close-out

**Files:**
- Modify: `docs/ROADMAP.md` — Phase 5 statuses: inbox items done; diff items done except image diffs (note "deferred — placeholder shipped"); palette done; tour done; "Did you know?" tooltips → won't-do; update the 5.1 lane list to the revised taxonomy.
- Modify: `docs/DESIGN.md` — replace the "Default lanes" section with the shipped 6-lane taxonomy.
- Modify: `CLAUDE.md` — only if commands/conventions changed (CodeView migration doesn't change any documented convention; likely no-op).

- [ ] **Step 1:** Update docs. Commit — `docs: Phase 5 roadmap and design close-out`

---

## Final verification (after all tasks)

```bash
cd go && go vet ./... && go test -race ./...
cd electron && pnpm type-check && pnpm lint && pnpm test
cd electron && pnpm start   # manual: inbox lanes, g-keys, Shift-R, split diff, w, generated collapse, :open in editor, tour
```
