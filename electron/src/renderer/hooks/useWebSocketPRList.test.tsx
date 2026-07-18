// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { initialUIState } from '../store/ui';
import type { PR } from '../types';
import { useWebSocketPRList } from './useWebSocketPRList';

const mockRequest = vi.hoisted(() => vi.fn());

vi.mock('./useWebSocket', () => ({
  useWebSocket: () => ({ request: mockRequest }),
}));

const makePR = (): PR => ({
  id: 'pr-1',
  provider: 'github',
  owner: 'acme',
  repo: 'api',
  number: 1,
  title: 'Test PR',
  body: null,
  author: 'alice',
  state: 'open',
  created_at: '2026-07-18T00:00:00Z',
  updated_at: '2026-07-18T00:00:00Z',
  merged_at: null,
  base_branch: 'main',
  head_branch: 'feature',
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
  mentioned: false,
  viewer_commented: false,
  unread: false,
  reviewed_head_stale: false,
  hydrated: 1,
});

function createWrapper(queryClient: QueryClient) {
  return function QueryWrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function renderPRListHook() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return renderHook(() => useWebSocketPRList(), {
    wrapper: createWrapper(queryClient),
  });
}

describe('useWebSocketPRList viewer login', () => {
  beforeEach(() => {
    appStore.setState(() => ({ ...initialUIState }));
    mockRequest.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('stores the viewer login from a PR list response', async () => {
    mockRequest.mockResolvedValue({
      prs: [makePR()],
      total: 1,
      has_more: false,
      viewer_login: 'alice',
    });

    renderPRListHook();

    await waitFor(() => {
      expect(appStore.state.viewerLogin).toBe('alice');
    });
  });

  it('resets the viewer login when a response omits it', async () => {
    appStore.setState((state) => ({ ...state, viewerLogin: 'alice' }));
    mockRequest.mockResolvedValue({
      prs: [],
      total: 0,
      has_more: false,
    });

    renderPRListHook();

    await waitFor(() => {
      expect(appStore.state.viewerLogin).toBe('');
    });
  });

  it('does not update the store when the viewer login is unchanged', async () => {
    appStore.setState((state) => ({ ...state, viewerLogin: 'alice' }));
    mockRequest.mockResolvedValue({
      prs: [],
      total: 0,
      has_more: false,
      viewer_login: 'alice',
    });
    const setStateSpy = vi.spyOn(appStore, 'setState');

    renderPRListHook();

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalledTimes(1);
    });
    await act(async () => undefined);

    expect(setStateSpy).not.toHaveBeenCalled();
  });
});
