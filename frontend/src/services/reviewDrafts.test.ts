import { afterEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { cancellable } from '../test/cancellable';
import { loadDrafts } from './reviewDrafts';

const List = vi.hoisted(() => vi.fn());
vi.mock('@bindings/services', () => ({
  Drafts: { List, Save: vi.fn(), Delete: vi.fn() },
}));

const draft = {
  id: 'd-1',
  pr_id: 'pr-1',
  path: 'go/main.go',
  side: 'RIGHT',
  start_line: 0,
  line: 12,
  body: 'nit',
  commit_sha: 'abc123',
  created_at: '2026-10-23T09:15:00Z',
};

describe('review drafts', () => {
  afterEach(() => {
    appStore.setState(() => initialUIState);
    List.mockReset();
  });

  it('stores the engine Drafts.List payload for the shown PR', async () => {
    appStore.setState((s) => ({ ...s, reviewDraftsPrId: 'pr-1' }));
    List.mockReturnValue(
      cancellable(Promise.resolve({ pr_id: 'pr-1', drafts: [draft] })),
    );

    const result = await loadDrafts('pr-1');

    expect(result.isOk()).toBe(true);
    expect(appStore.state.reviewDrafts).toEqual([draft]);
  });

  it('ignores a DRAFTS_READY reply for a PR that is no longer shown', async () => {
    appStore.setState((s) => ({ ...s, reviewDraftsPrId: 'pr-2' }));
    List.mockReturnValue(
      cancellable(Promise.resolve({ pr_id: 'pr-1', drafts: [draft] })),
    );

    const result = await loadDrafts('pr-1');

    expect(result.isOk()).toBe(true);
    expect(appStore.state.reviewDrafts).toEqual([]);
  });
});
