# Task 9: UI — triage filter on `s` and the pair picker; delete the range-diff UI

All paths below are relative to `electron/src/renderer/`.

**Files:**
- Create: `hooks/useTriage.ts`, `utils/sinceLastReview.ts`, `utils/sinceLastReview.test.ts`
- Modify: `types/index.ts`, `types/wsSchemas.ts`, `pages/DiffPage.tsx`, `pages/DiffPageChrome.tsx`, `components/diff/DiffView.tsx`, `components/diff/diffFileSidebar.tsx`, `utils/keybindings.ts` (description only)
- Delete: `hooks/useRangeDiff.ts`, `components/RangeDiffView.tsx`, `components/DegradedRangeDiffIndicator.tsx`

**Interfaces:**
- Consumes: the `REQUEST_TRIAGE` → `TRIAGE_READY` contract from Task 4 (see `task-04-triage-handler.md`), `splitUnifiedDiff` (`components/diff/diffPathUtils.ts`), `wsClient.request`, `parseResponse` (`utils/error.ts`).
- Produces (Task 10 relies on these):
  - `types/index.ts`: `triageResultSchema`, `type TriageResult`, `type TriageFileStatus`, `type TriageBaselineSource` (all via `z.infer`).
  - `hooks/useTriage.ts`: `type TriageRequest`, `useTriage(prId, req): { result: TriageResult | null; loading: boolean; error: string | null }`.
  - In `DiffPage.tsx`: `triageOk: TriageResult | null` (the result when `state === 'ok'`) and `currentFile` derived from the visible diff.

**Behavior:** `s` (or a selected `P2 → P5` pair) requests triage. When triage is `ok`, the diff shows only `changed`/`new` files, sidebar rows carry a status badge, removed files are listed in a banner, and the status bar reads `since P3 (your review, 2d ago): 4 of 37 files changed`. When triage is not `ok` (no baseline, no fingerprints, …) the full diff stays and the status bar shows the engine's reason. File bodies are always the normal current PR diff.

- [ ] **Step 1: Write the failing util test**

`utils/sinceLastReview.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { splitUnifiedDiff } from '../components/diff/diffPathUtils';
import type { TriageResult } from '../types';
import { filterDiffToPaths, triageSummary } from './sinceLastReview';

const NOW = new Date('2026-10-23T12:00:00Z');

function triage(overrides: Partial<TriageResult>): TriageResult {
  return {
    pr_id: 'github:o/r:1',
    state: 'ok',
    reason: '',
    from_patchset: 3,
    to_patchset: 5,
    baseline: null,
    files: [],
    changed_count: 4,
    total_count: 37,
    ...overrides,
  };
}

describe('triageSummary', () => {
  it('names the review baseline and its age', () => {
    const result = triage({
      baseline: { source: 'github_review', reviewed_at: '2026-10-21T09:00:00Z' },
    });
    expect(triageSummary(result, NOW)).toBe(
      'since P3 (your review, 2d ago): 4 of 37 files changed',
    );
  });

  it('names an all-viewed baseline', () => {
    const result = triage({
      baseline: { source: 'all_viewed', reviewed_at: '2026-10-23T09:00:00Z' },
    });
    expect(triageSummary(result, NOW)).toBe(
      'since P3 (all viewed, 3h ago): 4 of 37 files changed',
    );
  });

  it('labels an explicit pair', () => {
    expect(triageSummary(triage({}), NOW)).toBe('P3 → P5: 4 of 37 files changed');
  });

  it('shows the engine reason when triage is unavailable', () => {
    const result = triage({ state: 'no_baseline', reason: 'no prior review recorded' });
    expect(triageSummary(result, NOW)).toBe('no prior review recorded');
  });
});

describe('filterDiffToPaths', () => {
  it('keeps only the listed files', () => {
    const diff = [
      'diff --git a/a.go b/a.go',
      '--- a/a.go',
      '+++ b/a.go',
      '@@ -1 +1 @@',
      '-x',
      '+y',
      'diff --git a/b.go b/b.go',
      '--- a/b.go',
      '+++ b/b.go',
      '@@ -1 +1 @@',
      '-p',
      '+q',
      '',
    ].join('\n');

    const filtered = filterDiffToPaths(diff, new Set(['b.go']));

    expect(splitUnifiedDiff(filtered).map((file) => file.path)).toEqual(['b.go']);
  });
});
```

Run: `cd electron && pnpm vitest run src/renderer/utils/sinceLastReview.test.ts`
Expected: FAIL (module not found). If `pnpm vitest` isn't how this repo runs one file, check `electron/package.json` `test` script and use its runner with the file path.

- [ ] **Step 2: Types and schemas**

In `types/index.ts`, delete `RangeDiffResult` and `rangeDiffSchema` and add:

```ts
const triageFileStatusSchema = z.enum(['changed', 'new', 'removed', 'unchanged']);

/**
 * Result of a `REQUEST_TRIAGE` → `TRIAGE_READY` round-trip: each file's status
 * in `to_patchset` relative to `from_patchset`. `baseline` is the review event
 * the engine chose for since-last-review, `null` for an explicit pair. When
 * `state` is not `ok`, `files` is empty and `reason` says why.
 */
export const triageResultSchema = z.object({
  pr_id: z.string(),
  state: z.enum(['ok', 'no_patchsets', 'no_baseline', 'no_fingerprints']),
  reason: z.string(),
  from_patchset: z.number(),
  to_patchset: z.number(),
  baseline: z
    .object({
      source: z.enum(['github_review', 'bistro_action', 'all_viewed']),
      reviewed_at: z.string(),
    })
    .nullable(),
  files: z.array(z.object({ path: z.string(), status: triageFileStatusSchema })),
  changed_count: z.number(),
  total_count: z.number(),
});
export type TriageResult = z.infer<typeof triageResultSchema>;
export type TriageFileStatus = z.infer<typeof triageFileStatusSchema>;
export type TriageBaselineSource = NonNullable<TriageResult['baseline']>['source'];
```

In `types/wsSchemas.ts`, replace `RANGE_DIFF_READY: z.unknown(),` with `TRIAGE_READY: z.unknown(),` and `eventSchema('RANGE_DIFF_READY'),` with `eventSchema('TRIAGE_READY'),`.

- [ ] **Step 3: The util**

`utils/sinceLastReview.ts`:

```ts
import { splitUnifiedDiff } from '../components/diff/diffPathUtils';
import type { TriageBaselineSource, TriageResult } from '../types';

const MINUTE_MS = 60_000;
const HOUR_MS = 60 * MINUTE_MS;
const DAY_MS = 24 * HOUR_MS;

const BASELINE_LABELS = {
  github_review: 'your review',
  bistro_action: 'your review',
  all_viewed: 'all viewed',
} satisfies Record<TriageBaselineSource, string>;

/** Status-bar line for a triage result, e.g. `since P3 (your review, 2d ago): 4 of 37 files changed`. */
export function triageSummary(result: TriageResult, now: Date): string {
  if (result.state !== 'ok') return result.reason;
  const counts = `${result.changed_count} of ${result.total_count} files changed`;
  if (result.baseline === null) {
    return `P${result.from_patchset} → P${result.to_patchset}: ${counts}`;
  }
  const label = BASELINE_LABELS[result.baseline.source];
  const age = relativeAge(new Date(result.baseline.reviewed_at), now);
  return `since P${result.from_patchset} (${label}, ${age}): ${counts}`;
}

/** Keeps only the files of a unified diff whose path is in `paths`. */
export function filterDiffToPaths(diff: string, paths: ReadonlySet<string>): string {
  return splitUnifiedDiff(diff)
    .filter((file) => paths.has(file.path))
    .map((file) => file.patch)
    .join('\n');
}

function relativeAge(then: Date, now: Date): string {
  const elapsed = Math.max(0, now.getTime() - then.getTime());
  if (elapsed < MINUTE_MS) return 'just now';
  if (elapsed < HOUR_MS) return `${Math.floor(elapsed / MINUTE_MS)}m ago`;
  if (elapsed < DAY_MS) return `${Math.floor(elapsed / HOUR_MS)}h ago`;
  return `${Math.floor(elapsed / DAY_MS)}d ago`;
}
```

If `ParsedDiffFile.patch` already ends in `\n`, join with `''` instead so files don't gain blank lines; the test's round-trip through `splitUnifiedDiff` must pass either way.

Run: `cd electron && pnpm vitest run src/renderer/utils/sinceLastReview.test.ts`
Expected: PASS.

- [ ] **Step 4: `useTriage` replaces `useRangeDiff`**

`git rm electron/src/renderer/hooks/useRangeDiff.ts electron/src/renderer/components/RangeDiffView.tsx electron/src/renderer/components/DegradedRangeDiffIndicator.tsx`, then create `hooks/useTriage.ts`:

```ts
import { Fault } from '@thourum/fault';
import { useCallback, useEffect, useRef, useState } from 'react';
import { wsClient } from '../services/ws/connection';
import { type TriageResult, triageResultSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('useTriage');

/** Triage against the reviewer's last review, or an explicit patchset pair. */
export type TriageRequest =
  | { kind: 'sinceLastReview' }
  | { kind: 'pair'; fromPatchset: number; toPatchset: number };

interface TriageState {
  result: TriageResult | null;
  loading: boolean;
  error: string | null;
}

/**
 * Requests `REQUEST_TRIAGE` whenever `prId` or `req` changes; `req = null`
 * clears. A non-`ok` triage state is a normal result; only a transport or
 * parse failure populates `error`.
 */
export function useTriage(
  prId: string | undefined,
  req: TriageRequest | null,
): TriageState {
  const [state, setState] = useState<TriageState>({
    result: null,
    loading: false,
    error: null,
  });

  // `reqKey` is the real dependency; the live `req` is read through a ref so
  // the callback need not list each of its fields.
  const reqKey = req === null ? null : JSON.stringify(req);
  const reqRef = useRef(req);
  reqRef.current = req;

  const load = useCallback(() => {
    void reqKey;
    const current = reqRef.current;
    if (prId === undefined || current === null) {
      setState({ result: null, loading: false, error: null });
      return;
    }
    setState((s) => ({ ...s, loading: true, error: null }));
    const payload =
      current.kind === 'sinceLastReview'
        ? { pr_id: prId, from_patchset: 0, to_patchset: 0 }
        : {
            pr_id: prId,
            from_patchset: current.fromPatchset,
            to_patchset: current.toPatchset,
          };

    void wsClient
      .request('REQUEST_TRIAGE', payload)
      .mapErr((cause) => new Fault(cause).withTag('NETWORK_ERROR').withCause(cause))
      .andThen((data) => parseResponse(triageResultSchema, data))
      .match(
        (result) => {
          setState({ result, loading: false, error: null });
        },
        (fault) => {
          log.warn({ err: fault.message }, 'failed to load triage');
          setState({ result: null, loading: false, error: fault.message });
        },
      );
  }, [prId, reqKey]);

  useEffect(() => {
    load();
  }, [load]);

  return state;
}
```

- [ ] **Step 5: Sidebar badges**

`components/diff/diffFileSidebar.tsx`: add the prop and render a badge before the `+/-` counts.

```tsx
  /** Since-last-review status per path; badges are shown for non-unchanged files. */
  triageStatuses?: ReadonlyMap<string, TriageFileStatus>;
```

```tsx
                {triageStatus !== undefined && triageStatus !== 'unchanged' && (
                  <span className="font-mono text-[9px] uppercase tracking-widest text-primary shrink-0">
                    {triageStatus}
                  </span>
                )}
```

with `const triageStatus = triageStatuses?.get(file.path);` next to `isViewed`, and `import type { TriageFileStatus } from '../../types';`. In `DiffView.tsx` add the same optional `triageStatuses` prop (doc comment: "Since-last-review status per path, shown as sidebar badges.") and pass it to `DiffFileSidebar`.

- [ ] **Step 6: Chrome — toolbar label, status bar summary, no range body**

In `pages/DiffPageChrome.tsx`:
- Remove the `RangeDiffView` import and `RangeDiffResult` type import.
- `DiffPageToolbar`: replace `showRangeDiff`, `sinceLastReview`, `selectedPatchsetPair` with `triageLabel: string | null`. Render the existing bordered badge when `triageLabel !== null`, showing `triageLabel`. Always render the find input (it filters the sidebar in both modes).
- `DiffPageStatusBar`: replace `showRangeDiff` and `rangeResult` with `triageSummary: string | null`. Line count is always `diff.split('\n').length`. When `triageSummary !== null`, render `<span className="normal-case tracking-normal text-primary">{triageSummary}</span>` as the first item of the left group.
- `DiffPageBody`: remove `showRangeDiff`, `rangeLoading`, `rangeError`, `rangeResult` and the whole range branch; the full-diff branches stay.

- [ ] **Step 7: Wire `DiffPage`**

In `pages/DiffPage.tsx` (update the header doc comment: one diff surface; `s` filters to files changed since your last review), replace the range-diff block with:

```tsx
  // ── Triage (since last review, or an explicit patchset pair) ──────
  const sinceLastReview = confirmingAction === 'since-last-review';
  const triageReq = useMemo<TriageRequest | null>(() => {
    if (sinceLastReview) return { kind: 'sinceLastReview' };
    if (selectedPatchsetPair !== null) {
      const [from, to] = selectedPatchsetPair;
      if (from !== to) return { kind: 'pair', fromPatchset: from, toPatchset: to };
    }
    return null;
  }, [sinceLastReview, selectedPatchsetPair]);
  const { result: triage, loading: triageLoading, error: triageError } =
    useTriage(pr?.id, triageReq);
  const triageOk = triage?.state === 'ok' ? triage : null;

  const triageStatuses = useMemo(
    () =>
      triageOk === null
        ? undefined
        : new Map(triageOk.files.map((file) => [file.path, file.status])),
    [triageOk],
  );
  const visibleDiff = useMemo(() => {
    if (triageOk === null) return diff;
    const shown = triageOk.files
      .filter((file) => file.status === 'changed' || file.status === 'new')
      .map((file) => file.path);
    return filterDiffToPaths(diff, new Set(shown));
  }, [diff, triageOk]);
  const removedPaths =
    triageOk?.files.filter((file) => file.status === 'removed').map((file) => file.path) ?? [];

  const triageLabel =
    triageReq === null
      ? null
      : triageReq.kind === 'sinceLastReview'
        ? 'Since last review'
        : `P${triageReq.fromPatchset} → P${triageReq.toPatchset}`;
  const triageStatusText =
    triageReq === null
      ? null
      : triageLoading
        ? 'loading triage…'
        : triageError !== null
          ? `triage failed: ${triageError}`
          : triage === null
            ? null
            : triageSummary(triage, new Date());
```

`selectedFileIndex` indexes the files `DiffView` resolved (now from `visibleDiff`), so derive the current file by path:

```tsx
  const currentFile = useMemo(() => {
    const path = splitUnifiedDiff(visibleDiff)[selectedFileIndex]?.path;
    return files.find((file) => file.path === path);
  }, [visibleDiff, selectedFileIndex, files]);
```

Then: pass `diff={visibleDiff}` and `triageStatuses={triageStatuses}` to `DiffView`; pass `triageLabel` to the toolbar; pass `diff={visibleDiff}` and `triageSummary={triageStatusText}` to the status bar; drop the range props from `DiffPageBody`. Wrap `DiffView` so the removed-files banner sits above it:

```tsx
        <div className="h-full flex flex-col">
          {removedPaths.length > 0 && (
            <div className="shrink-0 px-4 py-1.5 border-b border-outline-variant/30 bg-surface-container-low font-mono text-[10px] text-outline">
              Removed since P{triageOk?.from_patchset}: {removedPaths.join(', ')}
            </div>
          )}
          <div className="flex-1 min-h-0">
            <DiffView … />
          </div>
        </div>
```

Imports: `useTriage`/`TriageRequest` from `../hooks/useTriage`, `filterDiffToPaths`/`triageSummary` from `../utils/sinceLastReview`, `splitUnifiedDiff` from `../components/diff/diffPathUtils`.

In `utils/keybindings.ts`, change the `diff.sinceLastReview` description to `'Since last review: changed files'`.

- [ ] **Step 8: Verify nothing references the range diff**

Run: `cd electron && grep -rn "RangeDiff\|rangeDiff\|RANGE_DIFF\|range diff" src`
Expected: no matches (update stray comments too).

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add -A electron/src/renderer
git commit -m "feat: triage files since last review on s

Replaces the range-diff view: s and the patchset pair picker filter the
normal diff to changed and new files, with sidebar badges and a summary."
```
