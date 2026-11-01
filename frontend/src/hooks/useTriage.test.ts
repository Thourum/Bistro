// @vitest-environment jsdom
import { Triage } from '@bindings/services';
import { act, createElement } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { cancellable } from '../test/cancellable';
import type { TriageResult } from '../types';
import { type TriageRequest, useTriage } from './useTriage';

vi.mock('@bindings/services', () => ({ Triage: { Get: vi.fn() } }));
vi.mock('../utils/logger', () => ({
  createLogger: () => ({ warn: vi.fn() }),
}));

it('clears prior triage and ignores responses after switching or disabling requests', async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const pendingResponses: Array<(result: TriageResult) => void> = [];
  vi.mocked(Triage.Get).mockImplementation(() =>
    cancellable(
      new Promise<TriageResult>((resolve) => pendingResponses.push(resolve)),
    ),
  );
  const root = createRoot(document.createElement('div'));
  let state: ReturnType<typeof useTriage> | undefined;
  function HookProbe({ request }: { request: TriageRequest | null }): null {
    state = useTriage('github:o/r:1', request);
    return null;
  }
  const render = async (request: TriageRequest | null): Promise<void> => {
    await act(async () => root.render(createElement(HookProbe, { request })));
  };
  const result: TriageResult = {
    pr_id: 'github:o/r:1',
    state: 'ok',
    reason: '',
    from_patchset: 3,
    to_patchset: 5,
    baseline: null,
    files: [{ path: 'a.go', status: 'changed' }],
    changed_count: 1,
    total_count: 1,
  };

  await render({ kind: 'sinceLastReview' });
  expect(Triage.Get).toHaveBeenLastCalledWith({
    pr_id: result.pr_id,
    from_patchset: 0,
    to_patchset: 0,
  });
  await act(async () => pendingResponses[0]?.(result));
  expect(state?.result).toEqual(result);

  await render({ kind: 'pair', fromPatchset: 2, toPatchset: 5 });
  expect(state).toEqual({ result: null, loading: true, error: null });
  await render({ kind: 'pair', fromPatchset: 3, toPatchset: 5 });
  await act(async () => pendingResponses[1]?.({ ...result, from_patchset: 2 }));
  expect(state).toEqual({ result: null, loading: true, error: null });

  await render(null);
  await act(async () => pendingResponses[2]?.(result));
  expect(state).toEqual({ result: null, loading: false, error: null });
  await act(async () => root.unmount());
  vi.unstubAllGlobals();
});
