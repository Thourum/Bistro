// @vitest-environment jsdom

import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { initialUIState } from '../store/ui';
import type { PR } from '../types';
import { useInboxReadActions } from './useInboxReadActions';

const mockMarkPRsRead = vi.hoisted(() => vi.fn());

vi.mock('./useMarkPRsRead', () => ({
  useMarkPRsRead: () => ({ markPRsRead: mockMarkPRsRead }),
}));

const makePR = (id: string): PR => ({
  id,
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
  unread: true,
  reviewed_head_stale: false,
  hydrated: 1,
});

describe('useInboxReadActions', () => {
  beforeEach(() => {
    appStore.setState(() => ({
      ...initialUIState,
      selectedPRIndex: 2,
      laneSectionsSnapshot: [
        { lane: 1, startIndex: 0, count: 2 },
        { lane: 2, startIndex: 2, count: 1 },
      ],
    }));
    settingsStore.setState((state) => ({
      ...state,
      prList: { ...state.prList, view: 'flat' },
    }));
    mockMarkPRsRead.mockReset();
    mockMarkPRsRead.mockResolvedValue({ ok: true, marked: 1 });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('marks only the filter-visible PRs in the selected lane', () => {
    renderHook(() =>
      useInboxReadActions([makePR('pr-1'), makePR('pr-2'), makePR('pr-3')]),
    );

    act(() => {
      document.dispatchEvent(new CustomEvent('bistro:inbox:mark-lane-read'));
    });

    expect(mockMarkPRsRead).toHaveBeenCalledWith(['pr-3']);
  });

  it('does nothing in grouped view', () => {
    settingsStore.setState((state) => ({
      ...state,
      prList: { ...state.prList, view: 'grouped' },
    }));
    renderHook(() => useInboxReadActions([makePR('pr-1')]));

    act(() => {
      document.dispatchEvent(new CustomEvent('bistro:inbox:mark-lane-read'));
    });

    expect(mockMarkPRsRead).not.toHaveBeenCalled();
  });

  it('removes its listener on unmount', () => {
    const { unmount } = renderHook(() => useInboxReadActions([makePR('pr-1')]));

    unmount();
    act(() => {
      document.dispatchEvent(new CustomEvent('bistro:inbox:mark-lane-read'));
    });

    expect(mockMarkPRsRead).not.toHaveBeenCalled();
  });

  it('uses the latest cursor and lane snapshot after rerendering', () => {
    const prs = [makePR('pr-1'), makePR('pr-2'), makePR('pr-3')];
    const { rerender } = renderHook(() => useInboxReadActions(prs));

    act(() => {
      appStore.setState((state) => ({
        ...state,
        selectedPRIndex: 0,
        laneSectionsSnapshot: [
          { lane: 1, startIndex: 0, count: 1 },
          { lane: 2, startIndex: 1, count: 2 },
        ],
      }));
    });
    rerender();

    act(() => {
      document.dispatchEvent(new CustomEvent('bistro:inbox:mark-lane-read'));
    });

    expect(mockMarkPRsRead).toHaveBeenCalledWith(['pr-1']);
  });
});
