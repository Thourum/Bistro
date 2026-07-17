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
import { FooterSlotProvider } from '../../src/renderer/components/FooterSlot';
import { OverlayProvider } from '../../src/renderer/overlays/OverlayContext';
import { PRDetailPage } from '../../src/renderer/pages/PRDetailPage';
import type { RouterContext } from '../../src/renderer/router';
import * as storeModule from '../../src/renderer/store';

const mockPR = {
  id: 'github-1',
  provider_id: 'github:default',
  owner: 'acme',
  repo: 'widgets',
  number: 42,
  title: 'Ship new feature',
  author: 'alice',
  state: 'open' as const,
  review_status: 'pending',
  created_at: '2025-04-08T10:00:00Z',
  updated_at: '2025-04-08T14:00:00Z',
  base_branch: 'main',
  head_branch: 'feat/new',
  additions: 10,
  deletions: 2,
  changed_files: 3,
  body: 'Adds new widget',
  hydrated: 1,
};

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [mockPR], isLoading: false, error: null }),
}));

const mockSend = vi.hoisted(() => vi.fn());
const mockRequest = vi.hoisted(() => vi.fn());
const mockRetry = vi.hoisted(() => vi.fn());
const hydrationState = vi.hoisted(() => ({
  isHydrating: false,
  error: null as string | null,
}));

vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: mockRequest,
    send: mockSend,
  }),
}));

vi.mock('../../src/renderer/hooks/usePRHydration', () => ({
  usePRHydration: () => ({ ...hydrationState, retry: mockRetry }),
}));

vi.mock('../../src/renderer/hooks/useReviewThreads', () => ({
  useReviewThreads: () => ({
    threads: [],
    loading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

vi.mock('../../src/renderer/hooks/useCIChecks', () => ({
  useCIChecks: () => ({ data: [], isLoading: false, error: null }),
}));

vi.mock('../../src/renderer/hooks/useComments', () => ({
  useComments: () => ({ data: [], isLoading: false, error: null }),
}));

vi.mock('../../src/renderer/hooks/useDiffFiles', () => ({
  useDiffFiles: () => ({ data: [], isLoading: false, error: null }),
}));

expect.extend(matchers);

const mockStore = {
  state: {
    selectedPRIndex: 0,
    filterState: 'open' as const,
    mode: 'detail' as const,
    confirmingAction: null,
    pendingAction: null,
    actionResult: null,
  },
  subscribe: vi.fn(() => () => {}),
  setState: vi.fn(),
};

function resetMockStoreState(): void {
  mockStore.state = {
    selectedPRIndex: 0,
    filterState: 'open' as const,
    mode: 'detail' as const,
    confirmingAction: null,
    pendingAction: null,
    actionResult: null,
  };
}

/**
 * Render PRDetailPage inside a minimal router with /prs/$prId route so
 * Route.useParams() can resolve the prId param.
 */
function renderWithPRRoute(prId: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  const rootRoute = createRootRouteWithContext<RouterContext>()({
    // Root must render Outlet so child routes are rendered.
    component: () => <Outlet />,
  });

  const prsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/prs',
    component: () => <Outlet />,
  });

  // The route path must match exactly what getRouteApi('/prs/$prId') expects.
  const prDetailRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '$prId',
    component: PRDetailPage,
  });

  const routeTree = rootRoute.addChildren([
    prsRoute.addChildren([prDetailRoute]),
  ]);

  const history = createMemoryHistory({
    initialEntries: [`/prs/${prId}`],
  });

  const testRouter = createRouter({
    routeTree,
    history,
    context: {
      queryClient,
      authChecked: true,
      isAuthenticated: true,
      onboardingComplete: true,
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <OverlayProvider>
        <FooterSlotProvider>
          <RouterProvider router={testRouter} />
        </FooterSlotProvider>
      </OverlayProvider>
    </QueryClientProvider>,
  );
}

describe('PRDetailPage', () => {
  beforeEach(() => {
    mockSend.mockClear();
    mockRequest.mockReset();
    mockRequest.mockResolvedValue({});
    mockRetry.mockReset();
    hydrationState.isHydrating = false;
    hydrationState.error = null;
    mockPR.hydrated = 1;
    mockStore.setState.mockClear();
    resetMockStoreState();
    vi.spyOn(storeModule, 'appStore', 'get').mockReturnValue(
      mockStore as unknown as typeof storeModule.appStore,
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the PR title', async () => {
    renderWithPRRoute('github-1');
    expect(await screen.findByText('Ship new feature')).toBeInTheDocument();
  });

  it('shows the SPACE-to-diff footer hint', async () => {
    renderWithPRRoute('github-1');
    expect(await screen.findByText(/view diff/i)).toBeInTheDocument();
  });

  it('waits for hydration before requesting PR details, and retries after hydration fails', async () => {
    mockPR.hydrated = 0;
    hydrationState.isHydrating = true;

    const { unmount } = renderWithPRRoute('github-1');

    expect(
      await screen.findByText('Preparing pull request details…'),
    ).toBeInTheDocument();
    expect(mockRequest).not.toHaveBeenCalledWith(
      'REQUEST_COMMENTS',
      expect.anything(),
    );
    expect(mockRequest).not.toHaveBeenCalledWith(
      'REQUEST_DIFF',
      expect.anything(),
    );
    expect(mockRequest).not.toHaveBeenCalledWith(
      'REQUEST_CI_CHECKS',
      expect.anything(),
    );

    unmount();
    hydrationState.isHydrating = false;
    hydrationState.error = 'Sync service unavailable';
    renderWithPRRoute('github-1');

    expect(
      await screen.findByText(
        'Failed to prepare pull request details: Sync service unavailable',
      ),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry hydration' }));

    expect(mockRetry).toHaveBeenCalledTimes(1);
    expect(mockRequest).not.toHaveBeenCalledWith(
      'REQUEST_COMMENTS',
      expect.anything(),
    );
    expect(mockRequest).not.toHaveBeenCalledWith(
      'REQUEST_DIFF',
      expect.anything(),
    );
    expect(mockRequest).not.toHaveBeenCalledWith(
      'REQUEST_CI_CHECKS',
      expect.anything(),
    );
  });

  it('shows not-found state when prId matches no PR', async () => {
    renderWithPRRoute('unknown-id-99');
    expect(await screen.findByTestId('pr-not-found')).toBeInTheDocument();
    expect(await screen.findByText(/PR not found/i)).toBeInTheDocument();
  });

  it('shows a focused request-changes composer and submits with Mod+Enter', async () => {
    mockStore.state.confirmingAction = 'request_changes';

    renderWithPRRoute('github-1');

    const textarea = await screen.findByRole('textbox', {
      name: /request changes comment/i,
    });
    expect(textarea).toHaveFocus();

    fireEvent.change(textarea, { target: { value: 'Please add tests.' } });
    fireEvent.keyDown(textarea, { key: 'Enter', metaKey: true });

    expect(mockSend).toHaveBeenCalledWith('REQUEST_CHANGES', {
      pr_id: 'github-1',
      comment: 'Please add tests.',
    });
  });

  it('cancels the request-changes composer with Escape', async () => {
    mockStore.state.confirmingAction = 'request_changes';

    renderWithPRRoute('github-1');

    const textarea = await screen.findByRole('textbox', {
      name: /request changes comment/i,
    });
    fireEvent.keyDown(textarea, { key: 'Escape' });

    expect(mockStore.setState).toHaveBeenCalledWith(expect.any(Function));
  });
});
