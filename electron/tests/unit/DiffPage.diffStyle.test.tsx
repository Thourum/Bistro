// @vitest-environment jsdom
//
// DiffPage owns the diff-style toggle write: the `Shift+S` keybinding only
// records a `toggle-diff-style` intent, and this page performs the settings
// write and surfaces failures as a toast.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import * as matchers from '@testing-library/jest-dom/matchers';
import { act, render, waitFor } from '@testing-library/react';
import { errAsync, okAsync } from 'neverthrow';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DiffPage } from '../../src/renderer/pages/DiffPage';
import type { RouterContext } from '../../src/renderer/router';
import { appStore } from '../../src/renderer/store';
import { settingsStore } from '../../src/renderer/store/settings';
import { initialUIState } from '../../src/renderer/store/ui';
import type { PR } from '../../src/renderer/types';
import { DEFAULT_SETTINGS } from '../../src/types/settings';

expect.extend(matchers);

const updateSettingsMock = vi.hoisted(() => vi.fn());

const pr: PR = {
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
  hydrated: 1,
};

vi.mock('../../src/renderer/store/settings', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('../../src/renderer/store/settings')>();
  return { ...actual, updateSettings: updateSettingsMock };
});

vi.mock('../../src/renderer/components/diff/DiffView', () => ({
  DiffView: () => null,
}));

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [pr], isLoading: false }),
}));

vi.mock('../../src/renderer/hooks/usePRHydration', () => ({
  usePRHydration: () => ({
    isHydrating: false,
    error: null,
    retry: vi.fn(),
  }),
}));

vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: vi.fn().mockResolvedValue({ diff: '', files: [], comments: [] }),
  }),
}));

vi.mock('../../src/renderer/hooks/useActionDispatch', () => ({
  useActionDispatch: () => ({ dispatchReplyComment: vi.fn() }),
}));

vi.mock('../../src/renderer/hooks/useMarkFileViewed', () => ({
  useMarkFileViewed: () => ({ markViewed: vi.fn() }),
}));

vi.mock('../../src/renderer/hooks/useExpertise', () => ({
  useExpertise: () => ({ expertise: null }),
}));

vi.mock('../../src/renderer/hooks/useReviewThreads', () => ({
  useReviewThreads: () => ({ threads: [] }),
}));

vi.mock('../../src/renderer/hooks/useRangeDiff', () => ({
  useRangeDiff: () => ({ result: null, loading: false, error: null }),
}));

function renderDiffPage(): void {
  const rootRoute = createRootRouteWithContext<RouterContext>()({
    component: () => <Outlet />,
  });
  const prsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/prs',
    component: () => <Outlet />,
  });
  const diffRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '$prId/diff',
    component: DiffPage,
  });
  const router = createRouter({
    routeTree: rootRoute.addChildren([prsRoute.addChildren([diffRoute])]),
    context: {} as RouterContext,
    history: createMemoryHistory({ initialEntries: ['/prs/github-1/diff'] }),
  });
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
}

describe('DiffPage diff-style toggle intent', () => {
  beforeEach(() => {
    updateSettingsMock.mockReset();
    updateSettingsMock.mockReturnValue(okAsync(undefined));
    appStore.setState(() => ({ ...initialUIState }));
    settingsStore.setState(() => ({
      ...DEFAULT_SETTINGS,
      diff: { style: 'unified' },
    }));
  });

  it('flips unified to split and clears the intent', async () => {
    renderDiffPage();

    act(() => {
      appStore.setState((s) => ({
        ...s,
        confirmingAction: 'toggle-diff-style',
      }));
    });

    await waitFor(() => {
      expect(updateSettingsMock).toHaveBeenCalledWith({
        diff: { style: 'split' },
      });
    });
    expect(appStore.state.confirmingAction).toBeNull();
    expect(appStore.state.toasts).toHaveLength(0);
  });

  it('flips split back to unified', async () => {
    settingsStore.setState((s) => ({ ...s, diff: { style: 'split' } }));
    renderDiffPage();

    act(() => {
      appStore.setState((s) => ({
        ...s,
        confirmingAction: 'toggle-diff-style',
      }));
    });

    await waitFor(() => {
      expect(updateSettingsMock).toHaveBeenCalledWith({
        diff: { style: 'unified' },
      });
    });
  });

  it('surfaces a settings write failure as a dismissible error toast', async () => {
    updateSettingsMock.mockReturnValue(errAsync('Settings are read-only'));
    renderDiffPage();

    act(() => {
      appStore.setState((s) => ({
        ...s,
        confirmingAction: 'toggle-diff-style',
      }));
    });

    await waitFor(() => {
      expect(appStore.state.toasts.at(-1)).toMatchObject({
        type: 'error',
        message: 'Settings are read-only',
        dismissible: true,
      });
    });
    expect(appStore.state.confirmingAction).toBeNull();
  });
});
