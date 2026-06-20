// @vitest-environment jsdom
//
// Slice 5 — /settings route integration tests
//
// Covers:
//   1. /settings deep-link renders SettingsPage (not collapsed to /prs by pin effect).
//   2. Cold-start /settings → mode:'settings'.
//   3. global.settings hotkey navigates to /settings.
//   4. Command-palette settings command navigates to /settings.
//   5. Settings escape navigates to /prs → mode:'list'.
//   6. Coherence: on /settings mode is enforced to 'settings'.
//   7. Leaving /settings to /prs → mode corrected to 'list'.
//   8. Overlay-close on /settings restores mode:'settings'.
//   9. Onboarding: when mode:'onboarding', OnboardingPage renders even though route is /prs.

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
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import React from 'react';
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
import type { RouterContext } from '../../src/renderer/router';
import { appStore } from '../../src/renderer/store';
import { initialUIState } from '../../src/renderer/store/ui';
import {
  DEFAULT_KEYBINDINGS,
  type KeyBinding,
} from '../../src/renderer/utils/keybindings';

expect.extend(matchers);

// ---------------------------------------------------------------------------
// Mocks
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
vi.mock('../../src/renderer/components/FooterSlot', () => ({
  FooterSlotProvider: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  FooterCenterSlot: ({ children }: { children: React.ReactNode }) => (
    <>{children}</>
  ),
  useFooterSlot: () => undefined,
}));
vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: [], isLoading: false, error: null }),
}));
vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({ ws: null, send: vi.fn(), request: vi.fn() }),
}));

// Page stubs
vi.mock('../../src/renderer/pages/PRDetailPage', () => ({
  PRDetailPage: () => <div data-testid="pr-detail-page">DetailPage</div>,
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

// Hoisted mock router for keybinding tests
const { mockRouterNavigate, mockRouterState } = vi.hoisted(() => {
  const mockRouterState = { location: { pathname: '/prs' } };
  return {
    mockRouterNavigate: vi.fn().mockResolvedValue(undefined),
    mockRouterState,
  };
});

vi.mock('../../src/renderer/router', () => ({
  router: {
    navigate: mockRouterNavigate,
    state: mockRouterState,
  },
  prDetailRoute: {},
  prDiffRoute: {},
}));

// ---------------------------------------------------------------------------
// Router factory
// ---------------------------------------------------------------------------

const prsIndexSearchSchema = z.object({
  filter: z.enum(['open', 'draft', 'all', 'needs_action']).default('open'),
  sort: z.enum(['updated', 'created']).default('updated'),
  q: z.string().default(''),
});

function makeSlice5Router(initialUrl: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  const rootRoute = createRootRouteWithContext<RouterContext>()({
    component: AuthedRouteShell,
  });

  const settingsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/settings',
    component: () => <div data-testid="settings-page">Settings</div>,
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
    component: () => <div data-testid="pr-list-page">PRListPage</div>,
  });

  const prDetailRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '$prId',
    component: () => <div data-testid="pr-detail-page">DetailPage</div>,
  });

  const routeTree = rootRoute.addChildren([
    settingsRoute,
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

/** Find a DEFAULT_KEYBINDINGS entry by actionId. */
function byId(actionId: string): KeyBinding {
  const b = DEFAULT_KEYBINDINGS.find((k) => k.actionId === actionId);
  if (!b) throw new Error(`no binding: ${actionId}`);
  return b;
}

/** Create a minimal KeyActionContext for testing binding actions. */
function makeTestCtx(
  routeBase: import('../../src/renderer/utils/keybindings').RouteBase | null = 'list',
): import('../../src/renderer/utils/keybindings').KeyActionContext {
  return {
    store: appStore,
    routeBase,
    activeOverlay: null,
    overlays: {
      activeOverlay: null,
      helpSourceRoute: 'list' as const,
      openCommand: vi.fn(),
      openHelp: vi.fn(),
      openSearch: vi.fn(),
      openFilter: vi.fn(),
      toggleFilter: vi.fn(),
      openRepoPicker: vi.fn(),
      closeOverlay: vi.fn(),
      closeSearch: vi.fn(),
    },
    location: { pathname: '/settings' },
  };
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  mockRouterNavigate.mockClear();
  mockRouterState.location.pathname = '/prs';
  appStore.setState(() => ({
    ...initialUIState,
    isAuthenticated: true,  }));
});

afterEach(() => {
  vi.restoreAllMocks();
  appStore.setState(() => ({ ...initialUIState }));
});

// ---------------------------------------------------------------------------
// 1. /settings deep-link renders SettingsPage (NOT collapsed to /prs)
// ---------------------------------------------------------------------------

describe('Slice 5 — /settings deep-link', () => {
  it('renders SettingsPage on deep-link /settings', async () => {
    
    makeSlice5Router('/settings');

    await waitFor(() => {
      expect(screen.getByTestId('settings-page')).toBeInTheDocument();
    });
    // PR list must NOT be visible
    expect(screen.queryByTestId('pr-list-page')).not.toBeInTheDocument();
  });

  it('/settings deep-link is NOT collapsed to /prs by pin effect', async () => {
    
    const { testRouter } = makeSlice5Router('/settings');

    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/settings');
    });
  });
});

// ---------------------------------------------------------------------------
// 2. Cold-start /settings — router renders SettingsPage (Slice 9: mode removed)
// ---------------------------------------------------------------------------

describe('Slice 5 — cold-start /settings renders SettingsPage', () => {
  it('navigating to /settings renders SettingsPage', async () => {
    makeSlice5Router('/settings');

    await waitFor(() => {
      expect(screen.getByTestId('settings-page')).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// 3. global.settings hotkey navigates to /settings
// ---------------------------------------------------------------------------

describe('Slice 5 — global.settings hotkey', () => {
  it('navigates to /settings (uses router.navigate)', () => {
    byId('global.settings').action(makeTestCtx('list'));

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/settings' }),
    );
  });
});

// ---------------------------------------------------------------------------
// 4. Command-palette settings command navigates to /settings
// ---------------------------------------------------------------------------
// Note: CommandPalette uses useNavigate() from react-router — we test the
// keybinding path for unit isolation; CommandPalette integration test covers
// the full UI path.

// ---------------------------------------------------------------------------
// 5. Settings escape navigates to /prs
// ---------------------------------------------------------------------------

describe('Slice 5 — settings escape', () => {
  it('Escape on settings navigates to /prs', () => {
    mockRouterState.location.pathname = '/settings';

    byId('global.escape').action(makeTestCtx('settings'));

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs' }),
    );
  });
});

// ---------------------------------------------------------------------------
// 6–8. Coherence tests removed — mode field deleted in Slice 9.
// Route ownership is 100% router-controlled; SettingsPage renders when on
// /settings. Routing behavior tested in test #2 above and slice8 guard tests.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 9. Onboarding: renders even though route is /prs (route-primary override)
// ---------------------------------------------------------------------------

// Slice 5 test #9 removed: onboarding is now a real route under RootShell
// (Slice 8). AuthedRouteShell no longer renders OnboardingPage — it is
// accessed via the /onboarding route with its own guard. This is covered by
// slice8-auth-guards.test.tsx instead.
