import { okAsync } from '@thourum/fault';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import { deleteDrafts, loadDrafts } from './reviewDrafts';

const request = vi.hoisted(() => vi.fn());
vi.mock('./ws/connection', () => ({
  wsClient: { send: vi.fn(), request },
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
    request.mockReset();
  });

  it('stores the engine DRAFTS_READY payload for the shown PR', async () => {
    appStore.setState((s) => ({ ...s, reviewDraftsPrId: 'pr-1' }));
    request.mockReturnValue(okAsync({ pr_id: 'pr-1', drafts: [draft] }));

    const result = await loadDrafts('pr-1');

    expect(result.isOk()).toBe(true);
    expect(appStore.state.reviewDrafts).toEqual([draft]);
  });

  it('fails with the engine error instead of reporting success', async () => {
    request.mockReturnValue(okAsync({ error: 'unable to delete drafts' }));

    const result = await deleteDrafts('pr-1', ['d-1']);

    expect(result.isErr() && result.error.message).toBe(
      'unable to delete drafts',
    );
  });
});
