// @vitest-environment jsdom

import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import type { PR } from '../types';
import { usePRHydration } from './usePRHydration';

const mockRequest = vi.hoisted(() => vi.fn());

vi.mock('./useWebSocket', () => ({
  useWebSocket: () => ({ request: mockRequest }),
}));

const listOnlyPR: PR = {
  id: 'github-1',
  provider: 'github:default',
  owner: 'acme',
  repo: 'widgets',
  number: 42,
  title: 'Ship new feature',
  body: null,
  author: 'alice',
  state: 'open',
  created_at: '2025-04-08T10:00:00Z',
  updated_at: '2025-04-08T14:00:00Z',
  merged_at: null,
  base_branch: 'main',
  head_branch: 'feature/new',
  base_sha: 'base',
  head_sha: 'head',
  comment_count: 0,
  ci_status: null,
  draft: 0,
  review_status: null,
  approved_by: null,
  changes_requested_by: null,
  requested_reviewers: [],
  touches_you: false,
  hydrated: 0,
};

describe('usePRHydration', () => {
  beforeEach(() => {
    mockRequest.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('surfaces an automatic hydration failure and retries only on explicit request', async () => {
    mockRequest
      .mockRejectedValueOnce(new Error('Sync service unavailable'))
      .mockResolvedValueOnce({});

    const { result } = renderHook(() => usePRHydration(listOnlyPR));

    await waitFor(() => {
      expect(result.current.error).toBe('Sync service unavailable');
    });

    expect(result.current.isHydrating).toBe(false);
    expect(mockRequest).toHaveBeenCalledTimes(1);
    expect(mockRequest).toHaveBeenLastCalledWith('REQUEST_PR_SYNC', {
      pr_id: listOnlyPR.id,
    });

    act(() => {
      result.current.retry();
    });

    await waitFor(() => {
      expect(result.current.isHydrating).toBe(false);
      expect(mockRequest).toHaveBeenCalledTimes(2);
    });

    expect(result.current.error).toBeNull();
  });

  it('does not request hydration for a PR that already has details', () => {
    mockRequest.mockResolvedValue({});

    const { result } = renderHook(() =>
      usePRHydration({ ...listOnlyPR, hydrated: 1 }),
    );

    expect(result.current).toEqual({
      isHydrating: false,
      error: null,
      retry: expect.any(Function),
    });
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it('keeps a single hydration request in flight when retry is invoked', async () => {
    mockRequest.mockImplementation(() => new Promise<void>(() => undefined));

    const { result } = renderHook(() => usePRHydration(listOnlyPR));

    await waitFor(() => {
      expect(result.current.isHydrating).toBe(true);
    });
    act(() => {
      result.current.retry();
      result.current.retry();
    });

    expect(mockRequest).toHaveBeenCalledTimes(1);
  });

  it('ignores a hydration result after the hook unmounts', async () => {
    let resolveRequest: (() => void) | undefined;
    mockRequest.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveRequest = resolve;
        }),
    );
    const setStateSpy = vi.spyOn(appStore, 'setState');
    const { unmount } = renderHook(() => usePRHydration(listOnlyPR));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledTimes(1);
    });
    unmount();

    await act(async () => {
      resolveRequest?.();
    });

    expect(setStateSpy).not.toHaveBeenCalled();
  });

  it('ignores a stale hydration result after the PR changes', async () => {
    let resolveRequest: (() => void) | undefined;
    mockRequest.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveRequest = resolve;
        }),
    );
    const setStateSpy = vi.spyOn(appStore, 'setState');
    const { rerender } = renderHook(
      ({ pr }: { pr: PR }) => usePRHydration(pr),
      {
        initialProps: { pr: listOnlyPR },
      },
    );

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledTimes(1);
    });
    rerender({ pr: { ...listOnlyPR, id: 'github-2', hydrated: 1 } });

    await act(async () => {
      resolveRequest?.();
    });

    expect(setStateSpy).not.toHaveBeenCalled();
  });
});
