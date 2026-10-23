# Task 10: UI — per-file interdiff on `i`

All paths below are relative to `electron/src/renderer/`.

**Files:**
- Create: `hooks/useInterdiff.ts`, `pages/DiffPageInterdiff.tsx`
- Modify: `types/index.ts`, `types/wsSchemas.ts`, `store/ui.ts`, `utils/keybindings.ts`, `utils/keybindingActions.ts`, `components/HelpOverlay.tsx`, `pages/DiffPage.tsx`, `pages/DiffPageChrome.tsx`, `pages/useDiffPageActions.ts`

**Interfaces:**
- Consumes: the `REQUEST_INTERDIFF` → `INTERDIFF_READY` contract from Task 8 (see `task-08-interdiff.md`); from Task 9: `triageOk`, `currentFile`, `triageStatuses`, `triageLabel` in `DiffPage.tsx`, the toolbar's `triageLabel` prop.
- Produces: `UIState.interdiffActive: boolean`, action `diff.interdiff` on `i`.

**Behavior:** With triage active and `ok`, `i` on a `changed` file swaps the body to that file's interdiff (baseline head → current head). The toolbar shows `interdiff P3 → P5`, plus `rebased: includes upstream changes` when the engine flags it. `i` again returns to the triage view. When `i` is on but the current file can't be interdiffed (no triage, file not `changed`), the normal view stays and the status bar says why. Moving between files with interdiff on shows the next file's interdiff. No new tests: the logic is a request/response pass-through, and the Go side is tested in Task 8.

- [ ] **Step 1: Schema**

`types/index.ts`:

```ts
/**
 * Result of a `REQUEST_INTERDIFF` → `INTERDIFF_READY` round-trip: one file at
 * the `from_patchset` head vs. the `to_patchset` head. `patch` is empty when
 * the file is identical at both heads; `available: false` carries a `reason`.
 */
export const interdiffResultSchema = z.object({
  pr_id: z.string(),
  path: z.string(),
  from_patchset: z.number(),
  to_patchset: z.number(),
  available: z.boolean(),
  reason: z.string(),
  patch: z.string(),
  rebased: z.boolean(),
});
export type InterdiffResult = z.infer<typeof interdiffResultSchema>;
```

`types/wsSchemas.ts`: add `INTERDIFF_READY: z.unknown(),` after `TRIAGE_READY` and `eventSchema('INTERDIFF_READY'),` after `eventSchema('TRIAGE_READY'),`.

- [ ] **Step 2: State, keybinding, action, help**

- `store/ui.ts`: add to `UIState`

  ```ts
    /** `i` in the diff view: show the current file's interdiff instead of the PR diff. */
    interdiffActive: boolean;
  ```

  and `interdiffActive: false,` to the initial state.
- `utils/keybindings.ts`, after `diff.sinceLastReview`:

  ```ts
    {
      actionId: 'diff.interdiff',
      keys: 'i',
      description: 'Interdiff: this file since last review',
      activation: DIFF,
    },
  ```

  First confirm no other DIFF-scoped or global binding uses `i` (`grep -n "keys: 'i'" utils/keybindings.ts`); `keybindings.test.ts` should catch conflicts.
- `utils/keybindingActions.ts`: add

  ```ts
  function toggleInterdiff(context: KeyActionContext): void {
    context.store.setState((state) => ({
      ...state,
      interdiffActive: !state.interdiffActive,
    }));
  }
  ```

  and `'diff.interdiff': toggleInterdiff,` in `ACTIONS` next to `'diff.sinceLastReview'`.
- `components/HelpOverlay.tsx`: add `'diff.interdiff'` to `DIFF_IDS` after `'diff.sinceLastReview'`.
- `pages/useDiffPageActions.ts`: in the unmount cleanup, also reset `interdiffActive`:

  ```ts
      appStore.setState((s) => ({
        ...s,
        interdiffActive: false,
        confirmingAction:
          s.confirmingAction === 'since-last-review' ? null : s.confirmingAction,
      }));
  ```

- [ ] **Step 3: `useInterdiff`**

`hooks/useInterdiff.ts`:

```ts
import { Fault } from '@thourum/fault';
import { useCallback, useEffect, useRef, useState } from 'react';
import { wsClient } from '../services/ws/connection';
import { type InterdiffResult, interdiffResultSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('useInterdiff');

/** One file between two patchsets; `toPatchset` 0 means latest. */
export interface InterdiffRequest {
  path: string;
  fromPatchset: number;
  toPatchset: number;
}

interface InterdiffState {
  result: InterdiffResult | null;
  loading: boolean;
  error: string | null;
}

/**
 * Requests `REQUEST_INTERDIFF` whenever `prId` or `req` changes; `req = null`
 * clears. `available: false` is a normal result; only a transport or parse
 * failure populates `error`.
 */
export function useInterdiff(
  prId: string | undefined,
  req: InterdiffRequest | null,
): InterdiffState {
  const [state, setState] = useState<InterdiffState>({
    result: null,
    loading: false,
    error: null,
  });

  // `reqKey` is the real dependency; the live `req` is read through a ref.
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
    void wsClient
      .request('REQUEST_INTERDIFF', {
        pr_id: prId,
        path: current.path,
        from_patchset: current.fromPatchset,
        to_patchset: current.toPatchset,
      })
      .mapErr((cause) => new Fault(cause).withTag('NETWORK_ERROR').withCause(cause))
      .andThen((data) => parseResponse(interdiffResultSchema, data))
      .match(
        (result) => {
          setState({ result, loading: false, error: null });
        },
        (fault) => {
          log.warn({ err: fault.message }, 'failed to load interdiff');
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

- [ ] **Step 4: The interdiff pane**

`pages/DiffPageInterdiff.tsx`:

```tsx
import { DiffView } from '../components/diff/DiffView';
import type { AppSettings, InterdiffResult } from '../types';

interface InterdiffPaneProps {
  result: InterdiffResult | null;
  loading: boolean;
  error: string | null;
  theme: Parameters<typeof DiffView>[0]['theme'];
  diffStyle: AppSettings['diff']['style'];
}

/** The current file between two patchset heads, in place of the PR diff. */
export function InterdiffPane({
  result,
  loading,
  error,
  theme,
  diffStyle,
}: InterdiffPaneProps): React.ReactElement {
  if (error !== null) return <PaneMessage tone="error">Failed to load interdiff: {error}</PaneMessage>;
  if (loading || result === null) return <PaneMessage>Loading interdiff…</PaneMessage>;
  if (!result.available) return <PaneMessage>Interdiff unavailable: {result.reason}</PaneMessage>;
  if (result.patch === '') {
    return (
      <PaneMessage>
        {result.path} is identical in P{result.from_patchset} and P{result.to_patchset}
      </PaneMessage>
    );
  }
  return <DiffView diff={result.patch} theme={theme} diffStyle={diffStyle} />;
}

function PaneMessage({
  tone = 'neutral',
  children,
}: {
  tone?: 'neutral' | 'error';
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <div
      className={`w-full h-full flex items-center justify-center font-mono text-xs ${
        tone === 'error' ? 'text-error' : 'text-on-surface-variant'
      }`}
    >
      {children}
    </div>
  );
}
```

Match `theme`'s type to what `DiffView`'s props declare (import that type directly if it's exported; `AppSettings` import path as used by `DiffView.tsx`).

- [ ] **Step 5: Wire `DiffPage` and the toolbar**

In `pages/DiffPage.tsx`, after `currentFile`:

```tsx
  const interdiffActive = useStore(appStore, (s: UIState) => s.interdiffActive);
  const canInterdiff =
    triageOk !== null &&
    currentFile !== undefined &&
    triageStatuses?.get(currentFile.path) === 'changed';
  const interdiffReq = useMemo<InterdiffRequest | null>(
    () =>
      interdiffActive && canInterdiff && triageOk !== null && currentFile !== undefined
        ? {
            path: currentFile.path,
            fromPatchset: triageOk.from_patchset,
            toPatchset: triageOk.to_patchset,
          }
        : null,
    [interdiffActive, canInterdiff, triageOk, currentFile],
  );
  const interdiff = useInterdiff(pr?.id, interdiffReq);
  const interdiffLabel =
    interdiffReq === null
      ? null
      : `interdiff P${interdiffReq.fromPatchset} → P${triageOk?.to_patchset}`;
  const interdiffHint =
    interdiffActive && !canInterdiff
      ? 'interdiff needs a changed file in triage (press s)'
      : null;
```

- Render `<InterdiffPane result={interdiff.result} loading={interdiff.loading} error={interdiff.error} theme={theme} diffStyle={diffStyle} />` in place of the banner + `DiffView` wrapper when `interdiffReq !== null`.
- Status bar: pass `triageSummary={interdiffHint ?? triageStatusText}`.
- Toolbar: add props `interdiffLabel: string | null` and `isRebased: boolean` (`interdiff.result?.rebased === true && interdiffReq !== null`). In `DiffPageToolbar`, when `interdiffLabel !== null` render it in the same bordered badge style next to the triage badge, and when `isRebased` add:

  ```tsx
            <span className="font-mono text-[10px] uppercase tracking-widest text-tertiary border border-tertiary/40 px-2 py-0.5">
              rebased: includes upstream changes
            </span>
  ```

  Use whichever warning/tertiary token the design system defines (`docs/DESIGN.md`, `tailwind` config) — no raw hex.
- Update the `DiffPage` header doc comment: `i` toggles the current file's interdiff.

- [ ] **Step 6: Verify**

Run: `cd electron && pnpm type-check && pnpm lint && pnpm test`
Expected: PASS.

Manual check (no component tests in this repo): `cd electron && pnpm start`, open a PR with ≥2 patchsets and a review baseline, press `s`, select a `changed` file, press `i` → interdiff shows; `i` → back to triage; `i` on a `new` file → status bar hint.

- [ ] **Step 7: Commit**

```bash
git add -A electron/src/renderer
git commit -m "feat: per-file interdiff on i in the triage view"
```
