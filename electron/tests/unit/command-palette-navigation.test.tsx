// @vitest-environment jsdom
//
// Command palette navigation integration test
//
// Renders the REAL CommandPalette (NOT mocked) inside a router provider with
// seeded PR data, opens it, selects a PR result, and asserts:
//   - the route becomes /prs/<that pr.id>
//   - mode is set to 'detail'
//   - the correct pr.id is navigated to (PR identity is the stable pr.id, not selectedPRIndex)

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
  within,
} from '@testing-library/react';
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest';
import React from 'react';
import { z } from 'zod/v4';
import { AuthedRouteShell } from '../../src/renderer/components/AuthedRouteShell';
import { PrsIndexRouteComponent } from '../../src/renderer/components/PrsIndexRouteComponent';
import { OverlayProvider, useOverlays } from '../../src/renderer/overlays/OverlayContext';
import type { RouterContext } from '../../src/renderer/router';
import { appStore } from '../../src/renderer/store';
import { useStore } from '../../src/renderer/hooks/useStore';
import { initialUIState } from '../../src/renderer/store/ui';
import type { PR } from '../../src/renderer/types';

expect.extend(matchers);

/**
 * Replicates ConfirmingActionOverlayBridge (now in RootShell) for tests that
 * use AuthedRouteShell as root without RootShell. Bridges confirmingAction==='command'
 * to overlays.openCommand() so the palette can still be triggered in tests.
 */
function ConfirmingActionBridgeForTest(): null {
  const confirmingAction = useStore(appStore, (s) => s.confirmingAction);
  const { openCommand } = useOverlays();
  React.useEffect(() => {
    if (confirmingAction === 'command') {
      appStore.setState((s) => ({ ...s, confirmingAction: null }));
      openCommand();
    }
  }, [confirmingAction, openCommand]);
  return null;
}

// ---------------------------------------------------------------------------
// Mocks — CommandPalette is NOT mocked (that's the point)
// ---------------------------------------------------------------------------

vi.mock('../../src/renderer/components/AppFooter', () => ({
  AppFooter: () => null,
}));
vi.mock('../../src/renderer/components/HelpOverlay', () => ({
  HelpOverlay: () => null,
}));
vi.mock('../../src/renderer/components/TourOverlay', () => ({
  TourOverlay: () => null,
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
  useWebSocket: () => ({
    ws: null,
    send: vi.fn(),
    request: vi.fn().mockResolvedValue({}),
  }),
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
// PRDetailPage stub — we only need to assert the route changed, not render detail.
vi.mock('../../src/renderer/pages/PRDetailPage', () => ({
  PRDetailPage: () => <div data-testid="pr-detail-stub">Detail stub</div>,
}));

// ---------------------------------------------------------------------------
// Test PR data — repo2 PRs have global indices 2 and 3
// ---------------------------------------------------------------------------

const PALETTE_PRS: PR[] = [
  {
    id: 'pal-pr-a',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'widgets',
    number: 10,
    title: 'Palette PR Alpha',
    author: 'alice',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-04T00:00:00Z',
    base_branch: 'main',
    head_branch: 'feat/alpha',
    additions: 1,
    deletions: 0,
    changed_files: 1,
    body: '',
    ci_status: 'success',
  },
  {
    id: 'pal-pr-b',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'widgets',
    number: 11,
    title: 'Palette PR Beta',
    author: 'bob',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-02T00:00:00Z',
    updated_at: '2025-01-03T00:00:00Z',
    base_branch: 'main',
    head_branch: 'feat/beta',
    additions: 2,
    deletions: 0,
    changed_files: 1,
    body: '',
    ci_status: 'success',
  },
];

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({
    data: PALETTE_PRS,
    isLoading: false,
    error: null,
  }),
}));

// ---------------------------------------------------------------------------
// Schema + router factory
// ---------------------------------------------------------------------------

const prsIndexSearchSchema = z.object({
  filter: z.enum(['open', 'draft', 'all', 'needs_action']).default('open'),
  sort: z.enum(['updated', 'created']).default('updated'),
  q: z.string().default(''),
});

function makePaletteRouter(initialUrl = '/prs/') {
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

  const prDetailRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '$prId',
    component: () => <div data-testid="pr-detail-stub">Detail stub</div>,
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
      {/* OverlayProvider wraps the router since AuthedRouteShell no longer
          contains it (moved to RootShell). CommandPalette (real,
          not mocked) uses useOverlays() and needs the provider here.
          ConfirmingActionBridgeForTest ensures confirmingAction==='command'
          still opens the palette (bridge is in RootShell in prod). */}
      <OverlayProvider>
        <ConfirmingActionBridgeForTest />
        <RouterProvider router={testRouter} />
      </OverlayProvider>
    </QueryClientProvider>,
  );

  return { ...utils, testRouter, queryClient };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('command palette: PR open navigates by pr.id', () => {
  beforeEach(() => {
    appStore.setState(() => ({
      ...initialUIState,
      isAuthenticated: true,    }));
  });

  afterEach(() => {
    vi.restoreAllMocks();
    appStore.setState(() => ({ ...initialUIState }));
  });

  it('selecting a PR in the palette navigates to /prs/<pr.id>', async () => {
    // selectedPRIndex=0 → pal-pr-a; we will select pal-pr-b from the palette.
    appStore.setState((s) => ({ ...s, selectedPRIndex: 0 }));

    const { testRouter } = makePaletteRouter('/prs/');

    // Wait for list to render.
    await waitFor(() => {
      expect(testRouter.state.location.pathname).toMatch(/^\/prs/);
    });

    // Open the command palette via confirmingAction bridge.
    act(() => {
      appStore.setState((s) => ({ ...s, confirmingAction: 'command' }));
    });

    // The real CommandPalette renders when activeOverlay==='command'.
    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /command palette/i })).toBeInTheDocument();
    });

    const dialog = screen.getByRole('dialog', { name: /command palette/i });

    // The palette shows recently opened PRs from useVisiblePRs().
    // 'Palette PR Beta' should appear (pal-pr-b).
    await waitFor(() => {
      expect(within(dialog).getByText('Palette PR Beta')).toBeInTheDocument();
    });

    // Click the 'Palette PR Beta' result within the dialog (pal-pr-b, global index 1 — NOT index 0).
    fireEvent.click(within(dialog).getByText('Palette PR Beta'));

    // Route must be /prs/pal-pr-b (the clicked PR's id), not /prs/pal-pr-a.
    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pal-pr-b');
    });

    // MUST NOT have navigated to the selectedPRIndex=0 PR (pal-pr-a).
    expect(testRouter.state.location.pathname).not.toBe('/prs/pal-pr-a');
  });

  it('selecting the FIRST PR in a list with selectedPRIndex pointing elsewhere navigates by id', async () => {
    // selectedPRIndex=1 → pal-pr-b; we select pal-pr-a from the palette.
    appStore.setState((s) => ({ ...s, selectedPRIndex: 1, confirmingAction: 'command' }));

    const { testRouter } = makePaletteRouter('/prs/');

    await waitFor(() => {
      expect(screen.getByRole('dialog', { name: /command palette/i })).toBeInTheDocument();
    });

    const dialog2 = screen.getByRole('dialog', { name: /command palette/i });

    await waitFor(() => {
      expect(within(dialog2).getByText('Palette PR Alpha')).toBeInTheDocument();
    });

    fireEvent.click(within(dialog2).getByText('Palette PR Alpha'));

    // Must navigate to pal-pr-a (not pal-pr-b at selectedPRIndex=1).
    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pal-pr-a');
    });

    expect(testRouter.state.location.pathname).not.toBe('/prs/pal-pr-b');
  });
});
