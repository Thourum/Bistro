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

**Integration point (verified):** `touches_you` is NOT computed at sync time — it's resolved at list-request time in `pr.go`. The correct write-side site is the sync writer: `go/internal/sync/writer.go`, which already post-processes comments per PR (see `writer_classify.go` usage in `writer_classify_test.go`). The writer has NO current-user dependency today; add one.

**Files:**
- Modify: `go/internal/sync/writer.go` (and `types.go` if the writer's constructor/deps struct lives there)
- Modify: `go/internal/sync/orchestrator.go` + `go/internal/server/app.go` — thread the dependency: inject the same `currentUserReader` interface the handlers use (`GetCurrentUser(ctx, "github")`, see `go/internal/server/handler/patchsets.go:~128`). Define a minimal local interface in the sync package (Go convention: consumer-side interfaces); pass the auth store at construction in `app.go`.
- Test: `go/internal/sync/writer_test.go` (append; the package has in-memory store test helpers — see `writer_classify_test.go`)

- [ ] **Step 1: Write failing test.** Seed a store with a PR whose body contains `hey @alice please look`, current user `alice` (stub `currentUserReader` returning `alice, true, nil`); run the writer over the PR+comments batch; assert `mentioned=1` in the DB. Cases: mention only in a comment; `@alice2` must NOT match `alice` (word boundary); case-insensitive `@Alice` matches; no current user → `mentioned` left 0, no error.

- [ ] **Step 2: Run it, confirm FAIL** (`go test ./internal/sync/ -run Mentioned -v`).

- [ ] **Step 3: Implement.** Regex `(?i)@` + `regexp.QuoteMeta(login)` + `\b`, compiled once per write batch. Match against `pr.Body` and the comment bodies the writer holds for the batch. `UPDATE prs SET mentioned = ? WHERE id = ?` inside the writer's existing transaction. **List-only guard:** list-only refreshes carry no comments, so an unconditional recompute would clear a comment-borne mention. Rule: on a write WITHOUT refreshed comments, never lower the flag — `UPDATE prs SET mentioned = MAX(mentioned, ?)` (body-only signal can only raise it); full/hydrating writes (comments refreshed) set it absolutely from body+comments. Regression test: hydrated PR mentioned via comment → list-only re-sync of the same PR → `mentioned` stays 1.

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

- [ ] **Step 5: Failing handler test** in `readstate_test.go`, copying the harness from an existing handler test (`comments_test.go` or `patchsets_test.go`): send `MARK_PRS_READ` with `{"pr_ids": ["p1","p2"]}`, assert ack `{ok: true, marked: 2}` and rows exist with fresh `last_read_at`. Error cases: empty `pr_ids` → error; no current user → `ok:false` skip (copy the `MARK_FILE_VIEWED` "no current user" behavior). **Timestamp comparability test:** insert a PR with `updated_at` written the way the sync writer writes it (Go `time.Time`), mark it read, assert the Task-4 unread expression evaluates false — this test fails if the two timestamp representations don't compare correctly in SQLite.

- [ ] **Step 6: Run, confirm FAIL.**

- [ ] **Step 7: Implement `NewMarkPRsReadHandler`** modeled on `NewMarkFileViewedHandler`:
  - Parse `{pr_ids []string}`; validate non-empty, cap at 500 ids.
  - Resolve current login the same way `MARK_FILE_VIEWED` does.
  - **Write `last_read_at` from Go, not SQLite:** bind `time.Now().UTC()` as the parameter so the stored representation matches how the sync writer stores `prs.updated_at` (both go through mattn/go-sqlite3 `time.Time` binding). Do NOT use `CURRENT_TIMESTAMP` (string form differs from Go-bound timestamps and breaks `<` comparison).
  - `INSERT INTO pr_read_state (pr_id, login, last_read_at) VALUES (?, ?, ?) ON CONFLICT(pr_id, login) DO UPDATE SET last_read_at = excluded.last_read_at` in one transaction.
  - Return `{ok: true, marked: N}`.
  - **Broadcaster dependency:** handlers don't receive the ws server directly, so declare a consumer-side interface in the handler package — `type broadcaster interface { Broadcast(msg interface{}) }` — and take it as a constructor param: `NewMarkPRsReadHandler(db prReadMarker, cu currentUserReader, bc broadcaster, log *slog.Logger)`. `*ws.Server` satisfies it. Tests use a spy broadcaster recording the pushed message.
  - Register in `app.go`: `a.wsServer.RegisterHandler("MARK_PRS_READ", handler.NewMarkPRsReadHandler(db, cu, a.wsServer, log))` next to `MARK_FILE_VIEWED`.
  - **After a successful write, broadcast an invalidation push** via that dependency: `{type: "PRS_READ_CHANGED", data: {pr_ids}}`. The renderer's PR-list hook refetches on it (Task 7). Test asserts the spy saw it (and saw nothing on the no-user path).

- [ ] **Step 8: Run handler tests + `go vet ./...`, confirm PASS.**

- [ ] **Step 9: Commit** — `feat(server): pr_read_state table and MARK_PRS_READ handler`

---

### Task 4: Extend `REQUEST_PR_LIST` payload — `mentioned`, `viewer_commented`, `unread`, `viewer_login`

**Files:**
- Modify: `go/internal/server/handler/pr.go` — **signature change:** `NewRequestPRListHandlerWithExpertise(db prLister, touchesYou touchesYouResolver, log)` gains a `cu currentUserReader` parameter (the same small interface `patchsets.go` declares — reuse or redeclare per Go consumer-interface convention). All existing constructors/tests updated.
- Modify: `go/internal/server/app.go` — pass the auth/current-user store at the registration site (same value `MARK_FILE_VIEWED` registration uses).
- Test: `go/internal/server/handler/pr_test.go` (or create; follow sibling handler tests) — existing fixtures updated for the new parameter (a stub `currentUserReader` returning a fixed login).

- [ ] **Step 1: Failing test.** Seed PRs + comments + read state; request `REQUEST_PR_LIST`; assert each PR row has `mentioned`, `viewer_commented`, `unread` booleans and the payload `data` carries `viewer_login`. Cases: PR with viewer comment → `viewer_commented: true`; PR with `updated_at` newer than `last_read_at` → `unread: true`; no read-state row → `unread: true`; marked read after update → `unread: false` (write `last_read_at` via the Task 3 handler path, NOT raw SQL, so the test exercises the real timestamp representation).

- [ ] **Step 2: Run, confirm FAIL** (compile error on signature first — fix fixtures, then behavior).

- [ ] **Step 3: Implement.**
  - Resolve login once per request via `cu.GetCurrentUser(ctx, "github")`.
  - Add to `prRow`: `Mentioned bool \`json:"mentioned"\``, `ViewerCommented bool \`json:"viewer_commented"\``, `Unread bool \`json:"unread"\``, `ReviewedHeadStale bool \`json:"reviewed_head_stale"\`` (true when a `last_reviewed_sha` row exists for (pr, viewer) and `sha != prs.head_sha` — subquery against the existing `last_reviewed_sha` table; add a test: review recorded at old sha + new head_sha → true; matching sha → false; no row → false).
  - Extend the SELECT (login passed as a bind param; keep parameterized — no string concat of the login). Compare timestamps with `unixepoch()` (SQLite ≥3.38, bundled with mattn/go-sqlite3) so representation differences can't break ordering:

```sql
SELECT ..., mentioned,
  EXISTS(SELECT 1 FROM comments c WHERE c.pr_id = prs.id AND c.author = ?) AS viewer_commented,
  (COALESCE((SELECT unixepoch(r.last_read_at) FROM pr_read_state r
             WHERE r.pr_id = prs.id AND r.login = ?), 0) < unixepoch(prs.updated_at)) AS unread,
  EXISTS(SELECT 1 FROM last_reviewed_sha lrs
         WHERE lrs.pr_id = prs.id AND lrs.login = ?
           AND lrs.sha != prs.head_sha) AS reviewed_head_stale
FROM prs WHERE 1=1 ...
```

(`last_reviewed_sha` is account-namespaced — check its PK/columns in `go/internal/store/db/last_reviewed.sql.go` and match the predicate to how `UpsertLastReviewedSHA` keys rows: if `account_provider`/`account_login` participate, bind them too, same values the executor writes.)

  - Add `"viewer_login": login` to the `PR_LIST_READY` `data` map.
  - When no current user is available, ship `mentioned` from the column, `viewer_commented:false`, `unread:false`, `viewer_login:""` (inbox degrades to FYI-heavy — acceptable pre-auth).

- [ ] **Step 4: Run `go test -race ./internal/server/...` + `go vet ./...`, confirm PASS.**

- [ ] **Step 5: Commit** — `feat(server): inbox signals in PR list payload`

---

### Task 5: Renderer lane model — `lanes.ts` + types

**Files:**
- Modify: `electron/src/renderer/types/index.ts` (PR interface: add `mentioned: boolean; viewer_commented: boolean; unread: boolean; reviewed_head_stale: boolean;` — all four Task-4 payload fields)
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
  reviewed_head_stale: false,
} as never; // build a PR factory matching the real PR type in the actual file

describe('laneForPR', () => {
  test('changes requested on your open PR → lane 1', () =>
    expect(laneForPR({ ...base, author: 'me', review_status: 'changes_requested' }, 'me')).toBe(1));
  test('you are a requested reviewer (open PR) → lane 1 (no age split)', () =>
    expect(laneForPR({ ...base, requested_reviewers: ['me'] }, 'me')).toBe(1));
  test('author pushed since your review → lane 1 (not waiting-on-author)', () =>
    expect(laneForPR({ ...base, changes_requested_by: 'me', reviewed_head_stale: true }, 'me')).toBe(1));
  test('CLOSED PR with review request never in lane 1 → falls through (lane 6 here)', () =>
    expect(laneForPR({ ...base, state: 'closed', requested_reviewers: ['me'] }, 'me')).toBe(6));
  test('MERGED PR you authored with changes_requested → lane 3, not lane 1', () =>
    expect(laneForPR({ ...base, state: 'merged', author: 'me', review_status: 'changes_requested' }, 'me')).toBe(3));
  test('you reviewed someone else's open PR, no new pushes → lane 2', () =>
    expect(laneForPR({ ...base, changes_requested_by: 'me' }, 'me')).toBe(2));
  test('you commented on someone else's open PR → lane 2', () =>
    expect(laneForPR({ ...base, viewer_commented: true }, 'me')).toBe(2));
  test('closed PR you commented on → NOT lane 2 (falls through)', () =>
    expect(laneForPR({ ...base, state: 'closed', viewer_commented: true }, 'me')).toBe(6));
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
  const isOpen = pr.state === 'open';
  if (isViewer) {
    const reviewed =
      pr.approved_by === viewerLogin || pr.changes_requested_by === viewerLogin;
    // Lane 1 requires an OPEN PR: stale signals on closed/merged PRs fall through.
    if (isOpen) {
      if (isAuthor && pr.review_status === 'changes_requested') return 1;
      if (pr.requested_reviewers.includes(viewerLogin)) return 1;
      // Author pushed since your review → actionable again.
      if (!isAuthor && reviewed && pr.reviewed_head_stale) return 1;
    }
    if (!isAuthor && isOpen && (reviewed || pr.viewer_commented)) return 2;
    if (isAuthor) return 3;
  }
  if (pr.touches_you) return 4;
  if (pr.mentioned) return 5;
  return 6;
}
```

- [ ] **Step 4: Run tests, confirm PASS.** Also `pnpm type-check`.

- [ ] **Step 5: Wire payload fields**: add the four booleans (`mentioned`, `viewer_commented`, `unread`, `reviewed_head_stale`) to the `PR` interface; in `useWebSocketPRList.ts`, read `viewer_login` from `PR_LIST_READY` data and write it to the app store (`viewerLogin`, default `''`, in `store/ui.ts`).

- [ ] **Step 6: `pnpm type-check && pnpm lint && pnpm test`, all green. Commit** — `feat(inbox): lane assignment model`

---

### Task 6: Inbox UI — lane sections, unread styling, ordering

**View-mode reality check (verified):** `prList.view` exists in settings but NOTHING consumes it today — `PRListTable.tsx` and `PRListPage.tsx` always project repo-grouped rows through `visiblePRListRows` (repo `<details>` groups + `collapsedRepoKeys`). This task makes `prList.view` real: `'flat'` (default) renders lane sections, `'grouped'` renders the existing repo groups. Both the table AND the page's cursor-row projection must switch on it, or `selectedPRIndex`/`j`/`k`/`Enter` desync from what's on screen.

**Cursor/header design (the load-bearing decision):** the keyboard cursor NEVER sees headers. Two layers, strictly separated:

1. **Cursor layer (unchanged contract):** `useVisiblePRs().prs` stays a PR-only array; `selectedPRIndex` indexes it; `j`/`k` stay plain ±1; `Enter` stays `prs[selectedPRIndex]`. NO skip logic anywhere. The only Task-6 change at this layer is the `'flat'`-view sort: `(lane asc, updated_at desc)`.
2. **Presentation layer:** lane headers are rendered BETWEEN PR rows by the table alone. Because the list is lane-sorted, the table derives header positions with a pure helper:

```typescript
// utils/lanes.ts (add alongside laneForPR)
/** Positions in a lane-sorted PR list where each lane's header renders.
 *  Returns all six lanes; empty lanes carry startIndex of the next lane
 *  (header renders with count 0 and a '— clear —' row). */
export function laneSections(
  prs: PR[], viewerLogin: string,
): { lane: LaneId; startIndex: number; count: number }[]
```

The table iterates `laneSections` and slices `prs[startIndex, startIndex+count)` under each header. `globalIndex` passed to `PRRow` is the index in `prs` — identical to today's contract, so selection highlighting, `Enter`, and `Shift+G` need zero changes.

3. **Lane jumps (`g N`):** `selectedPRIndex = laneSections(prs, login)[N-1].startIndex`, no-op when `count === 0`. **Current lane for `Shift-R`:** the section whose `[startIndex, startIndex+count)` contains `selectedPRIndex`. Both are pure functions over the same `laneSections` output — one source of truth, no drift.

`'grouped'` view: `visiblePRListRows` (repo groups) is used exactly as today; lane sections don't exist there, and the lane keybindings no-op (guard on view mode in the action).

**Files:**
- Modify: `electron/src/renderer/hooks/useVisiblePRs.ts` — `'flat'`-view sort by `(lane, updated_at DESC)` (lane from `laneForPR(pr, viewerLogin)`); `'grouped'` unchanged.
- Modify: `electron/src/renderer/utils/lanes.ts` — add `laneSections` + tests (empty lanes, all-in-one-lane, interleaved input gets sorted upstream so sections are contiguous).
- Modify: `electron/src/renderer/components/PRListTable.tsx` — `'flat'`: render lane headers from `laneSections` (`<laneId> <LABEL> — <count>`, existing uppercase/mono style; empty lane: header + dimmed `— clear —` row); `'grouped'`: existing `visiblePRListRows` path untouched. Unread rows: title `font-semibold` + `●` prefix; read rows: existing dimmed styles.
- Modify: `electron/src/renderer/pages/PRListPage.tsx` — cursor math already reads the PR-only list; verify nothing there assumes repo-group rows in `'flat'` view.
- Read first: where the settings value reaches the renderer (`grep -rn 'prList' electron/src/renderer --include='*.ts*'` — likely via `useSettingsEffect.ts`/app store); wire it if it isn't already in the store.
- Modify: the Settings page (`electron/src/renderer/pages/` — find the settings route component) — **add the user-facing view toggle**: a "PR list view: Lanes / Repo groups" control writing `prList.view`, styled like the neighboring settings controls (nothing consumes or sets this field today, so without this control the retained grouped view is unreachable). Plus a `:view lanes` / `:view groups` palette command pair wired to the same setting (cheap, keyboard-first).
- Test: `electron/src/renderer/utils/lanes.test.ts` (laneSections — including `startIndex` correctness for leading, middle, and trailing EMPTY lanes, since `g N` jumps depend on those indexes) + `electron/src/renderer/components/PRListTable.test.tsx` (extend existing tests) + settings-page test for the toggle if the page has a test file

- [ ] **Step 1: Failing component test:** in `'flat'` view, given PRs mapped to lanes 1/4/6, the table renders six headers with counts `1 NEEDS ACTION — 1`, `4 YOUR CODE — 1`, `6 FYI — 1`, empty lanes show `— clear —`, an unread PR row shows the `●` marker; in `'grouped'` view, repo groups render exactly as before (existing tests keep passing).

- [ ] **Step 2: Run, confirm FAIL.**

- [ ] **Step 3: Implement** (keep `useVisiblePRs` the single source of truth; lane grouping is presentation over the flat sorted list — compute `laneForPR` once per PR in a `useMemo`, don't re-derive per row).

- [ ] **Step 4: Tests + type-check + lint green. Verify keyboard invariant:** `j`/`k` from row N lands on the PR the highlight shows, across a lane boundary, in both views.

- [ ] **Step 5: Manual check:** `cd electron && pnpm start`, verify lanes render, switch `prList.view` to `grouped` in settings and verify repo groups still work.

- [ ] **Step 6: Commit** — `feat(inbox): lane-sectioned PR list with unread markers`

---

### Task 7: Inbox keyboard — `g 1`…`g 6`, `Shift-R`, auto-read on open

**Files:**
- Modify: `electron/src/renderer/utils/keybindings.ts` (bindings; sequences like `['g','1']` are supported — see `list.top: ['g','g']`)
- Create: `electron/src/renderer/hooks/useMarkPRsRead.ts` (copy `useMarkFileViewed.ts` pattern; message `MARK_PRS_READ`, payload `{pr_ids}`; on ack, refetch/invalidate the PR list the same way other mutations do)
- Modify: `electron/src/renderer/components/PRListTable.tsx` or the open-navigation site — send `MARK_PRS_READ` for a PR when it is opened
- Test: `electron/src/renderer/utils/keybindings.test.ts` if binding-level tests exist (check first); otherwise cover via `lanes.test.ts` helpers + hook test

**Bridge constraint (verified):** keybinding actions are pure config receiving only `KeyActionContext` (store/router/overlays) — they CANNOT call React hooks. The existing bridge pattern is DOM CustomEvents: `emitThreadsEvent('bistro:comments:dismiss-nits')` in `keybindings.ts` + a listener in `useDismissals.ts`. Reuse exactly that pattern.

- [ ] **Step 1:** Add bindings config (test-first where a test harness exists):
  - `inbox.lane1`…`inbox.lane6`: keys `['g','1']`…`['g','6']`, route `list`, hidden: lanes 2–6 (avoid help-overlay noise, same convention as jump keys), description "Jump to lane N". **Bridge for lane data:** `KeyActionContext` has no PR list, so `PRListPage` publishes the current sections into the store whenever the visible list changes: `laneSectionsSnapshot: {lane, startIndex, count}[]` in `UIState` (immutable, replaced wholesale in the same effect that already syncs `visiblePRIds`; empty array in `'grouped'` view). Action: read `laneSectionsSnapshot[N-1]` from the store; `selectedPRIndex = startIndex` (no-op when snapshot is empty — grouped view — or the lane's `count === 0`). `Shift-R`'s listener resolves "current lane" from the same snapshot.
  - `inbox.markLaneRead`: keys `'Shift+R'`, route `list`. Action: `document.dispatchEvent(new CustomEvent('bistro:inbox:mark-lane-read'))` (same helper as `emitThreadsEvent`). NOTE: `Shift+R` is `detail.requestChanges` on the `detail` route — route gating keeps them disjoint; verify with the help overlay.
- [ ] **Step 2:** Implement `useMarkPRsRead` (WS request hook per `useMarkFileViewed` pattern) plus a listener hook `useInboxReadActions` mounted in the list page: on `bistro:inbox:mark-lane-read`, compute filter-visible PRs in the selected row's lane (from `useVisiblePRs` + `laneForPR`) and call `markPRsRead(ids)`; no-op when the list is empty.
- [ ] **Step 3:** **Refetch on push:** subscribe `useWebSocketPRList.ts` to the `PRS_READ_CHANGED` broadcast (Task 3) and refetch/invalidate the list. Check how existing broadcasts (e.g. `SYNC_COMPLETE`) reach hooks first — `useWebSocket`'s `request/send` API may need a `subscribe(type, cb)` (or the message-event listener pattern the sync-status hook uses); extend the shared hook, don't hand-roll a second socket listener.
- [ ] **Step 4:** **Auto-read on open, centralized:** send `MARK_PRS_READ` from the PR detail route component on mount (keyed by `prId`) — NOT from the row's `onClick` — so keyboard `Enter`, mouse click, palette open, and deep links all mark read through the one code path. Fire-and-forget with neverthrow (`ResultAsync`, no `.catch`/`try`). Offline note: `MARK_PRS_READ` is read-state, not a GitHub write — it does NOT go through the offline action queue; if the WS is down the mark is dropped and the PR simply stays unread until next opened (accepted; document in the hook comment).
- [ ] **Step 5:** `pnpm type-check && pnpm lint && pnpm test` green. Manual: `g 3` jumps, `Shift-R` clears bold for the lane, opening a PR (each of: Enter, click, palette) clears its dot.
- [ ] **Step 6: Commit** — `feat(inbox): lane jump keys, Shift-R mark-read, auto-read on open`

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

- [ ] **Step 1:** Write `diffPathUtils.test.ts` — the path helpers are private in `PierreDiffView.tsx`, so do a minimal extraction FIRST (move the pure functions verbatim to `diffPathUtils.ts`, re-import them in `PierreDiffView.tsx`), then write the characterization tests against the new module — rename returns NEW path, deletion falls back to a/ side, same-path headers. Run: PASS (locks behavior).
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
- Modify: `electron/src/renderer/components/CommandPalette.tsx` (command entry, `diff`-route-aware; **wording per spec: "Open file in editor"** — no "at current line" promise; description mentions the selected-comment-line behavior)
- Create: `electron/src/renderer/hooks/useOpenInEditor.ts` (WS calls + dialog state)
- Create: `electron/src/renderer/utils/editorArgs.ts` + `editorArgs.test.ts` (pure CLI argument builder — testable without Electron)
- Modify: `electron/src/main/ipc.ts` — add `open-in-editor` IPC handler (sibling of `openExternal`) that spawns the editor
- Modify: `electron/src/main/preload.ts` — expose `openInEditor(repoRoot, relativePath, line)` on the context bridge (context isolation is on; the renderer cannot reach `ipcRenderer` directly — follow how `openExternal` is exposed here). The renderer passes the `local_path` it got from `CHECK_PR_CHECKOUT` as `repoRoot` plus the repo-relative file path; **main performs the containment check itself**: `path.resolve(repoRoot, relativePath)` must satisfy `resolved === root || resolved.startsWith(root + path.sep)` after resolving — the `+ path.sep` guard rejects sibling-prefix bypasses like `/repo-evil` passing a `/repo` check. Tests cover `../` traversal and the sibling-prefix case.
- Modify: the renderer's window/bridge type declaration (find with `grep -rn 'openExternal' electron/src/renderer electron/src/types` — add `openInEditor` beside it)
- Test: `useOpenInEditor.test.ts` for the decision logic (mock WS + bridge), `editorArgs.test.ts` for the arg builder

Current file/line model (spec): file = `n`/`p` focus target; line = selected inline-comment line, else first new-side line of the file's first hunk, else 1.

- [ ] **Step 1: Failing tests:** (a) `editorArgs.test.ts` — pure builder: `code`/`cursor` → `['-g', 'file:line']`; unknown editor → `[file]` without line; empty editor name → error Result. (b) `useOpenInEditor.test.ts` — given `{head_matches: true}` → invokes bridge with launch args; `{head_matches: false}` → surfaces dialog state with three options; "no local path" error → toast text. neverthrow `ResultAsync` chains, no try/catch.
- [ ] **Step 2:** Run — FAIL. Implement `editorArgs.ts` + hook.
- [ ] **Step 3:** Main-process IPC (`ipc.ts`) + preload bridge (`preload.ts`) + window type: handler resolves `$EDITOR` (unset → typed error the renderer toasts as "Set $EDITOR"); validates the resolved file path stays inside the repo local path (reject `..` traversal) before spawning; spawns with an args array (no shell string).
- [ ] **Step 4:** Dialog component: "PR revision not checked out" → [Check out PR head] (calls `CHECKOUT_PR_HEAD`, then opens), [Open current version] (opens with mismatch warning in dialog body), [Cancel]. Reuse the existing dialog primitives (`components/ui/`).
- [ ] **Step 5:** All checks green; manual end-to-end with a locally-checked-out repo. Commit — `feat(palette): open in editor with checkout verification`

---

### Task 18: First-run tour

**Files:**
- Modify: `electron/src/renderer/overlays/OverlayContext.tsx` — add `'tour'` to `OverlayKind` AND expose an open path the same way existing overlays do (check how `help`/`command` open: the context's open/setActiveOverlay API — mirror it for `tour`)
- Modify: `electron/src/renderer/utils/keybindings.ts` — add `'tour'` to the hard-coded `ALL_OVERLAYS` list (line ~131) so the global Escape binding fires inside the tour; tour-internal keys (arrows/Enter) are handled in the component, matching how other overlays self-handle
- Create: `electron/src/renderer/overlays/TourOverlay.tsx` (5 screens as a const array; arrow/enter advances, Esc dismisses)
- Modify: `electron/src/types/settings.ts` — `appearance.tourComplete: boolean` default `false` (sibling of `onboardingComplete`)
- Modify: `electron/src/renderer/components/CommandPalette.tsx` — `:tour` command re-opens it
- Modify: app shell (where onboarding gating lives — `grep -rn 'onboardingComplete' electron/src/renderer`) — auto-open tour once after onboarding completes
- Test: `TourOverlay.test.tsx`

Screens (content, keys rendered from `keybindings.ts` config via the same lookup the help overlay uses — no hardcoded key names; check `HelpOverlay.tsx` for the accessor). Route-dependent keys carry a context label so the same key on two screens reads as deliberate (spec §5.4):
1. Inbox & lanes (`g 1`–`g 6`, `Shift-R` — "mark lane read *(in the inbox)*")
2. Navigation (`j`/`k`/`Enter`, `/` search, `f` filter)
3. Diff review (`n`/`p`, `Shift+S` split, `w` whitespace, `v` viewed, `s` since-last-review)
4. Actions (`c` comment, `a a` approve, `Shift+R` — "request changes *(on a PR)*", `d n`/`d b` dismiss)
5. Palette & help (`:` commands, `?` bindings)

- [ ] **Step 1: Failing tests:** renders screen 1 of 5; advances; Esc closes and persists `tourComplete: true`; advancing past screen 5 (normal completion) ALSO closes and persists `tourComplete: true`; shows override key when a binding is overridden.
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
