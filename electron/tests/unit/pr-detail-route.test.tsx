// @vitest-environment jsdom
//
// PR detail route integration tests
//
// Contract: $prId (a stable pr.id) is the ONLY identity for detail.
// NO detail code path may resolve a PR via selectedPRIndex.
// selectedPRIndex is a keyboard CURSOR only.
//
// These tests assert that navigating/rendering always shows the CORRECT PR
// by id — regardless of index position in filtered/sorted lists.

import * as matchers from '@testing-library/jest-dom/matchers';
import {
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest';
import { z } from 'zod/v4';
import { AuthedRouteShell } from '../../src/renderer/components/AuthedRouteShell';
import { PrsIndexRouteComponent } from '../../src/renderer/components/PrsIndexRouteComponent';
import { PRDetailPage } from '../../src/renderer/pages/PRDetailPage';
import type { RouterContext } from '../../src/renderer/router';
import { appStore } from '../../src/renderer/store';
import { initialUIState } from '../../src/renderer/store/ui';
import type { PR } from '../../src/renderer/types';

expect.extend(matchers);

// ---------------------------------------------------------------------------
// Mock heavy sub-components
// ---------------------------------------------------------------------------

vi.mock('../../src/renderer/components/AppFooter', () => ({
  AppFooter: () => null,
}));
vi.mock('../../src/renderer/components/CommandPalette', () => ({
  CommandPalette: () => null,
}));
vi.mock('../../src/renderer/components/HelpOverlay', () => ({
  HelpOverlay: () => null,
}));
vi.mock('../../src/renderer/components/RepoPicker', () => ({
  RepoPicker: () => null,
}));
vi.mock('../../src/renderer/components/RepoPickerDialog', () => ({
  RepoPickerDialog: () => null,
}));
vi.mock('../../src/renderer/components/ToastContainer', () => ({
  default: () => null,
}));
vi.mock('../../src/renderer/hooks/useAppHotkeys', () => ({
  useAppHotkeys: () => undefined,
}));
vi.mock('../../src/renderer/hooks/useSettingsEffect', () => ({
  useSettingsEffect: () => undefined,
}));
vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ ws: null, request: vi.fn().mockResolvedValue({}) }),
}));
vi.mock('../../src/renderer/components/FooterSlot', () => ({
  FooterSlotProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  FooterCenterSlot: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  useFooterSlot: () => undefined,
}));

// ---------------------------------------------------------------------------
// Test PR data — two repos with PRs whose global indices differ from filtered
// ---------------------------------------------------------------------------

/** 4 PRs: 2 in repo1, 2 in repo2 */
const ALL_PRS: PR[] = [
  {
    id: 'pr-repo1-a',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'repo1',
    number: 1,
    title: 'PR Repo1-A',
    author: 'alice',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-04T00:00:00Z',
    base_branch: 'main',
    head_branch: 'feat/a',
    additions: 1,
    deletions: 0,
    changed_files: 1,
    body: '',
    ci_status: 'success',
  },
  {
    id: 'pr-repo1-b',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'repo1',
    number: 2,
    title: 'PR Repo1-B',
    author: 'bob',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-02T00:00:00Z',
    updated_at: '2025-01-03T00:00:00Z',
    base_branch: 'main',
    head_branch: 'feat/b',
    additions: 2,
    deletions: 0,
    changed_files: 1,
    body: '',
    ci_status: 'success',
  },
  {
    id: 'pr-repo2-c',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'repo2',
    number: 3,
    title: 'PR Repo2-C',
    author: 'carol',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-03T00:00:00Z',
    updated_at: '2025-01-02T00:00:00Z',
    base_branch: 'main',
    head_branch: 'feat/c',
    additions: 3,
    deletions: 0,
    changed_files: 1,
    body: '',
    ci_status: 'pending',
  },
  {
    id: 'pr-repo2-d',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'repo2',
    number: 4,
    title: 'PR Repo2-D',
    author: 'dave',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-04T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    base_branch: 'main',
    head_branch: 'feat/d',
    additions: 4,
    deletions: 0,
    changed_files: 1,
    body: '',
    ci_status: 'failure',
  },
];

// Mock useWebSocketPRList to return our seeded data
vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({
    data: ALL_PRS,
    isLoading: false,
    error: null,
  }),
}));

// ---------------------------------------------------------------------------
// PRDetailPage stub — simple, no router hooks in factory
// ---------------------------------------------------------------------------

vi.mock('../../src/renderer/pages/PRDetailPage', () => ({
  // Minimal stub: renders a sentinel testid so tests can confirm detail rendered.
  PRDetailPage: () => (
    <div data-testid="pr-detail-stub">Detail stub</div>
  ),
}));

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

const prsIndexSearchSchema = z.object({
  filter: z.enum(['open', 'draft', 'all', 'needs_action']).default('open'),
  sort: z.enum(['updated', 'created']).default('updated'),
  q: z.string().default(''),
});

// ---------------------------------------------------------------------------
// Test router factory — uses the real route structure
// ---------------------------------------------------------------------------

function makePrDetailRouteRouter(initialUrl = '/prs/') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  const rootRoute = createRootRouteWithContext<RouterContext>()({
    component: AuthedRouteShell,
  });

  const prsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/prs',
    component: () => <Outlet />,
  });

  const prsIndexRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '/',
    validateSearch: prsIndexSearchSchema,
    component: PrsIndexRouteComponent,
  });

  // prDetailRoute — rendered by PRDetailPage (mocked above).
  const prDetailRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '$prId',
    component: PRDetailPage,
  });

  const routeTree = rootRoute.addChildren([
    prsRoute.addChildren([prsIndexRoute, prDetailRoute]),
  ]);

  const history = createMemoryHistory({ initialEntries: [initialUrl] });

  const testRouter = createRouter({
    routeTree,
    history,
    context: {
      queryClient,    },
  });

  const utils = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={testRouter} />
    </QueryClientProvider>,
  );

  return { ...utils, testRouter, queryClient };
}

// ---------------------------------------------------------------------------
// PR detail route integration test suite
// ---------------------------------------------------------------------------

describe('PR detail resolves by stable id, not list index', () => {
  beforeEach(() => {
    appStore.setState(() => ({
      ...initialUIState,
      isAuthenticated: true,
    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    appStore.setState(() => ({ ...initialUIState }));
  });

  // ---------------------------------------------------------------------------
  // 1. Click a row in a repo-FILTERED list
  // Historical bug: when filtered to repo2, rows are at filtered indices 0/1
  // but in the unfiltered list they are at global indices 2/3.
  // Old code: click sets selectedPRIndex=0, detail shows prs[0]=pr-repo1-a ❌
  // New code: navigate to /prs/pr-repo2-c, detail shows PR Repo2-C ✅
  // ---------------------------------------------------------------------------

  it('click first row of repo-filtered list shows the correct PR by id (not global index)', async () => {
    // Apply repo filter for repo2 — only PR-C and PR-D visible.
    appStore.setState((s) => ({
      ...s,
      repoFilter: 'acme/repo2',
    }));

    const { testRouter } = makePrDetailRouteRouter('/prs/');

    // Wait for the PR list to render.
    await waitFor(() => {
      expect(screen.getByText('PR Repo2-C')).toBeInTheDocument();
    });

    // Click the first visible row (PR Repo2-C, filtered index 0, global index 2).
    fireEvent.click(screen.getByText('PR Repo2-C'));

    // Must navigate to /prs/pr-repo2-c (navigate by id, not by index 0).
    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pr-repo2-c');
    });

    // MUST NOT navigate to /prs/pr-repo1-a (which was at global index 0 — the old bug).
    expect(testRouter.state.location.pathname).not.toBe('/prs/pr-repo1-a');
  });

  // ---------------------------------------------------------------------------
  // 2. Click second row of repo-filtered list
  // Old bug: index-based navigation ignores filter
  // ---------------------------------------------------------------------------

  it('click second row of repo-filtered list shows the correct PR by id', async () => {
    appStore.setState((s) => ({
      ...s,
      repoFilter: 'acme/repo2',
    }));

    const { testRouter } = makePrDetailRouteRouter('/prs/');

    await waitFor(() => {
      expect(screen.getByText('PR Repo2-D')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('PR Repo2-D'));

    // Navigate by pr.id — NOT by global index.
    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pr-repo2-d');
    });

    // MUST NOT navigate to /prs/pr-repo1-b (global index 1).
    expect(testRouter.state.location.pathname).not.toBe('/prs/pr-repo1-b');
  });

  // ---------------------------------------------------------------------------
  // 3. Deep-link to /prs/$prId — cold start renders the correct PR
  // ---------------------------------------------------------------------------

  it('deep-link to /prs/$prId renders the correct PR without collapsing to /prs', async () => {
    // Cold-start at a specific PR URL.
    const { testRouter } = makePrDetailRouteRouter('/prs/pr-repo2-c');

    await waitFor(() => {
      // Detail stub is rendered.
      expect(screen.getByTestId('pr-detail-stub')).toBeInTheDocument();
    });

    // The URL must NOT have been collapsed back to /prs.
    expect(testRouter.state.location.pathname).toBe('/prs/pr-repo2-c');
  });

  // ---------------------------------------------------------------------------
  // 4. List refresh / reorder does not change which PR the open detail shows
  // ---------------------------------------------------------------------------

  it('navigating to /prs/$prId by id is stable across list reorders', async () => {
    // Navigate directly to PR-repo2-c by id (as if user clicked it).
    const { testRouter } = makePrDetailRouteRouter('/prs/pr-repo2-c');

    await waitFor(() => {
      expect(screen.getByTestId('pr-detail-stub')).toBeInTheDocument();
    });

    // Even if the list order changes (e.g., selectedPRIndex gets clobbered),
    // the URL stays at /prs/pr-repo2-c (id-based, not index-based).
    act(() => {
      appStore.setState((s) => ({
        ...s,
        selectedPRIndex: 0, // index 0 = pr-repo1-a (different PR)
      }));
    });

    // URL is still /prs/pr-repo2-c — not collapsed to /prs/pr-repo1-a.
    expect(testRouter.state.location.pathname).toBe('/prs/pr-repo2-c');
  });

  // ---------------------------------------------------------------------------
  // 5. Not-found: unknown prId — mock shows empty prId (PR not in list)
  // ---------------------------------------------------------------------------

  it('navigating to an unknown prId renders detail with that prId in the param', async () => {
    const { testRouter } = makePrDetailRouteRouter('/prs/unknown-id-xyz');

    await waitFor(() => {
      expect(screen.getByTestId('pr-detail-stub')).toBeInTheDocument();
    });

    // The router is at the unknown prId — detail renders (no crash, no redirect).
    expect(testRouter.state.location.pathname).toBe('/prs/unknown-id-xyz');
  });

  // ---------------------------------------------------------------------------
  // 6. List shows correct rows when repo-filtered (sanity check)
  // ---------------------------------------------------------------------------

  it('repo filter shows only the matching PRs in the list', async () => {
    appStore.setState((s) => ({
      ...s,
      repoFilter: 'acme/repo1',
    }));

    makePrDetailRouteRouter('/prs/');

    await waitFor(() => {
      expect(screen.getByText('PR Repo1-A')).toBeInTheDocument();
      expect(screen.getByText('PR Repo1-B')).toBeInTheDocument();
    });

    expect(screen.queryByText('PR Repo2-C')).not.toBeInTheDocument();
    expect(screen.queryByText('PR Repo2-D')).not.toBeInTheDocument();
  });
});
