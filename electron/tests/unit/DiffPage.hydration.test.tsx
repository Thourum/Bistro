// @vitest-environment jsdom

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
import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DiffPage } from '../../src/renderer/pages/DiffPage';
import type { RouterContext } from '../../src/renderer/router';
import type { PR } from '../../src/renderer/types';

expect.extend(matchers);

const mockRequest = vi.hoisted(() => vi.fn());
const mockRetry = vi.hoisted(() => vi.fn());
const hydrationState = vi.hoisted(() => ({
  isHydrating: true,
  error: null as string | null,
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

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [listOnlyPR], isLoading: false }),
}));

vi.mock('../../src/renderer/hooks/usePRHydration', () => ({
  usePRHydration: () => ({ ...hydrationState, retry: mockRetry }),
}));

vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ request: mockRequest }),
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

describe('DiffPage hydration', () => {
  beforeEach(() => {
    mockRequest.mockReset();
    mockRequest.mockResolvedValue({ comments: [] });
    mockRetry.mockReset();
    hydrationState.isHydrating = true;
    hydrationState.error = null;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('keeps the diff route open without requesting a diff while hydration is pending', async () => {
    renderDiffPage();

    expect(
      await screen.findByText('Preparing pull request details…'),
    ).toBeInTheDocument();
    expect(mockRequest).not.toHaveBeenCalledWith(
      'REQUEST_DIFF',
      expect.anything(),
    );
  });

  it('shows the hydration error and provides an explicit retry path', async () => {
    hydrationState.isHydrating = false;
    hydrationState.error = 'Sync service unavailable';

    renderDiffPage();

    expect(
      await screen.findByText(
        'Failed to prepare pull request details: Sync service unavailable',
      ),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry hydration' }));

    expect(mockRetry).toHaveBeenCalledTimes(1);
    expect(mockRequest).not.toHaveBeenCalledWith(
      'REQUEST_DIFF',
      expect.anything(),
    );
  });
});
