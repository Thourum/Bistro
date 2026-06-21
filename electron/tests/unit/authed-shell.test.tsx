// @vitest-environment jsdom
//
// Authed shell routing tests:
// 1. mode='detail' → PRDetailPage rendered, PRListPage NOT rendered
// 2. mode='list' → PRListPage rendered (via Outlet), PRDetailPage NOT rendered
// 3. Sync: initial URL /prs/?filter=draft&sort=created&q=foo hydrates store
// 4. Sync: store write updates router search params

import * as matchers from '@testing-library/jest-dom/matchers';
import {
  act,
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
import { appStore } from '../../src/renderer/store';
import { initialUIState } from '../../src/renderer/store/ui';

expect.extend(matchers);

// ---------------------------------------------------------------------------
// Mock heavy sub-components so we only test shell routing logic
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
  useWebSocket: () => ({ ws: null, request: vi.fn() }),
}));

// Lightweight page stubs with testid markers
vi.mock('../../src/renderer/pages/PRDetailPage', () => ({
  PRDetailPage: () => (
    <div data-testid="pr-detail-page">DetailPage</div>
  ),
}));
vi.mock('../../src/renderer/pages/DiffPage', () => ({
  DiffPage: () => <div data-testid="diff-page">DiffPage</div>,
}));
vi.mock('../../src/renderer/pages/OnboardingPage', () => ({
  OnboardingPage: () => <div data-testid="onboarding-page">Onboarding</div>,
}));
vi.mock('../../src/renderer/pages/SettingsPage', () => ({
  SettingsPage: () => <div data-testid="settings-page">Settings</div>,
}));

// Stub PRListPage (real component; we render via Outlet)
vi.mock('../../src/renderer/pages/PRListPage', () => ({
  PRListPage: ({
    routeSearch,
  }: {
    routeSearch?: { filter: string; sort: string; q: string };
  }) => (
    <div data-testid="pr-list-page" data-filter={routeSearch?.filter ?? ''}>
      PRListPage
    </div>
  ),
}));
vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [], isLoading: false, error: null }),
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
// Test router factory — creates a fresh isolated router for each test
// ---------------------------------------------------------------------------

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  createMemoryHistory,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from '@tanstack/react-router';
import React from 'react';
import { z } from 'zod/v4';
import { AuthedRouteShell } from '../../src/renderer/components/AuthedRouteShell';
import { PrsIndexRouteComponent } from '../../src/renderer/components/PrsIndexRouteComponent';
import type { RouterContext } from '../../src/renderer/router';

const prsIndexSearchSchema = z.object({
  filter: z.enum(['open', 'draft', 'all', 'needs_action']).default('open'),
  sort: z.enum(['updated', 'created']).default('updated'),
  q: z.string().default(''),
});

function makeTestRouter(initialUrl: string) {
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

  // prDetailRoute — renders PRDetailPage via Outlet.
  const prDetailRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '$prId',
    component: () => <div data-testid="pr-detail-page">DetailPage</div>,
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
// Tests
// ---------------------------------------------------------------------------

describe('AuthedRouteShell — route-based rendering', () => {
  beforeEach(() => {
    // Reset store to defaults before each test
    appStore.setState(() => ({ ...initialUIState, isAuthenticated: true, authChecked: true }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    appStore.setState(() => ({ ...initialUIState }));
  });

  it('renders PRDetailPage when navigated to /prs/$prId; PRListPage NOT rendered', async () => {
    // Detail is router-owned — navigate to /prs/$prId.
    const { queryByTestId } = makeTestRouter('/prs/some-pr-id');

    // PRDetailPage stub is rendered by the prDetailRoute via Outlet.
    await waitFor(() => {
      expect(queryByTestId('pr-detail-page')).toBeInTheDocument();
    });
    // PRListPage is NOT rendered (different route).
    expect(queryByTestId('pr-list-page')).not.toBeInTheDocument();
  });

  it('renders PRListPage when navigated to /prs/', async () => {
    const { queryByTestId } = makeTestRouter('/prs/');

    await waitFor(() => {
      expect(queryByTestId('pr-list-page')).toBeInTheDocument();
    });
    expect(queryByTestId('pr-detail-page')).not.toBeInTheDocument();
  });

  it('deep-link to /prs/$prId is NOT collapsed back to /prs by the pin effect', async () => {
    const { testRouter } = makeTestRouter('/prs/some-pr-id');

    await waitFor(() => {
      const { pathname } = testRouter.state.location;
      // The pin effect must preserve the deep-linked path.
      expect(pathname).toBe('/prs/some-pr-id');
    });
  });
});

describe('usePrsSearchStoreBridge — route↔store sync', () => {
  beforeEach(() => {
    appStore.setState(() => ({
      ...initialUIState,
      isAuthenticated: true,    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    appStore.setState(() => ({ ...initialUIState }));
  });

  it('deep-link URL hydrates store on mount', async () => {
    makeTestRouter('/prs/?filter=draft&sort=created&q=foo');

    await waitFor(() => {
      expect(appStore.state.filterState).toBe('draft');
      expect(appStore.state.sortBy).toBe('created');
      expect(appStore.state.searchQuery).toBe('foo');
    });
  });

  it('store write updates router search params', async () => {
    const { testRouter } = makeTestRouter('/prs/?filter=open&sort=updated&q=');

    // Wait for initial hydration
    await waitFor(() => {
      expect(appStore.state.filterState).toBe('open');
    });

    // Simulate FilterMenu writing the store
    act(() => {
      appStore.setState((s) => ({ ...s, filterState: 'all' }));
    });

    await waitFor(() => {
      // location.search is a parsed object in TanStack Router
      const searchObj = testRouter.state.location.search as Record<string, unknown>;
      expect(searchObj['filter']).toBe('all');
    });
  });
});
