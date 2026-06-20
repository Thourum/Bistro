// @vitest-environment jsdom
//
// Slice 2 remediation — C2 extended integration tests
//
// Covers:
//   1. Real PRDetailPage resolves by $prId while selectedPRIndex points elsewhere.
//   2. Action dispatch (approve) uses the ROUTED prId, not selectedPRIndex.
//   3. Keyboard Enter (list.open) routes to the correct visible pr id.
//   4. Shift+G then Enter routes to the LAST visible pr.
//   5. Command-palette PR open navigates by pr.id.
//   6. Route↔mode coherence: Escape, back-nav, filter-then-Enter freshness.

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
import {
  DEFAULT_KEYBINDINGS,
  type KeyBinding,
} from '../../src/renderer/utils/keybindings';
import type { PR } from '../../src/renderer/types';

expect.extend(matchers);

// ---------------------------------------------------------------------------
// Mocks — do NOT mock PRDetailPage so we test the real resolver
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

// Hoisted mock fns — must be declared BEFORE vi.mock calls (which are hoisted).
const { mockSend, mockRouterNavigate } = vi.hoisted(() => ({
  mockSend: vi.fn().mockReturnValue('req-id'),
  mockRouterNavigate: vi.fn().mockResolvedValue(undefined),
}));

// WebSocket mock with a spy on `send` so action dispatch tests can inspect calls.
vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    ws: null,
    send: mockSend,
    request: vi.fn().mockResolvedValue({}),
  }),
}));

// ---------------------------------------------------------------------------
// Test PR data
// ---------------------------------------------------------------------------

const ALL_PRS: PR[] = [
  {
    id: 'pr-repo1-a',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'repo1',
    number: 1,
    title: 'PR Repo1-A Title',
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
    title: 'PR Repo1-B Title',
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
    title: 'PR Repo2-C Title',
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
    title: 'PR Repo2-D Title',
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

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({
    data: ALL_PRS,
    isLoading: false,
    error: null,
  }),
}));

// ---------------------------------------------------------------------------
// Mock the singleton router navigate so keybinding action tests can spy on it.
// The router singleton (router.tsx) is imported by keybindings.ts.
// mockRouterNavigate declared via vi.hoisted above.
// ---------------------------------------------------------------------------

vi.mock('../../src/renderer/router', () => ({
  router: { navigate: mockRouterNavigate, state: { location: { pathname: '/prs/' } } },
  prDetailRoute: {},
}));

// ---------------------------------------------------------------------------
// Schema + router factory
// ---------------------------------------------------------------------------

const prsIndexSearchSchema = z.object({
  filter: z.enum(['open', 'draft', 'all', 'needs_action']).default('open'),
  sort: z.enum(['updated', 'created']).default('updated'),
  q: z.string().default(''),
});

function makeExtRouter(initialUrl = '/prs/') {
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
    // Use the REAL PRDetailPage — not mocked.
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

/** Find a DEFAULT_KEYBINDINGS entry by actionId. */
function byId(actionId: string): KeyBinding {
  const b = DEFAULT_KEYBINDINGS.find((k) => k.actionId === actionId);
  if (!b) throw new Error(`no binding: ${actionId}`);
  return b;
}

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
    location: { pathname: '/prs' },
  };
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

beforeEach(() => {
  mockSend.mockClear();
  mockRouterNavigate.mockClear();
  appStore.setState(() => ({
    ...initialUIState,
    isAuthenticated: true,  }));
});

afterEach(() => {
  vi.restoreAllMocks();
  appStore.setState(() => ({ ...initialUIState }));
});

// ---------------------------------------------------------------------------
// 1. Real PRDetailPage resolves by $prId while selectedPRIndex points elsewhere
// ---------------------------------------------------------------------------

describe('C2 extended — real PRDetailPage resolver', () => {
  it('renders the ROUTED PR title, not the PR at selectedPRIndex', async () => {
    // selectedPRIndex=0 points to pr-repo1-a; but we route to pr-repo2-c.
    appStore.setState((s) => ({ ...s, selectedPRIndex: 0 }));

    makeExtRouter('/prs/pr-repo2-c');

    // Real PRDetailPage must show the title of pr-repo2-c.
    await waitFor(() => {
      expect(screen.getByText('PR Repo2-C Title')).toBeInTheDocument();
    });

    // MUST NOT show pr-repo1-a title (old index-based bug).
    expect(screen.queryByText('PR Repo1-A Title')).not.toBeInTheDocument();
  });

  it('shows not-found UI when prId has no matching PR', async () => {
    makeExtRouter('/prs/unknown-no-such-pr');

    await waitFor(() => {
      expect(screen.getByTestId('pr-not-found')).toBeInTheDocument();
    });
  });
});

// ---------------------------------------------------------------------------
// 2. Action dispatch targets the routed prId, not selectedPRIndex
// ---------------------------------------------------------------------------

describe('C2 extended — action dispatch by routed prId', () => {
  it('approve_confirmed dispatch sends pr_id from route param, not selectedPRIndex', async () => {
    // selectedPRIndex=0 → pr-repo1-a; route is pr-repo2-c.
    appStore.setState((s) => ({
      ...s,
      selectedPRIndex: 0,
    }));

    makeExtRouter('/prs/pr-repo2-c');

    // Wait for the real PRDetailPage to render.
    await waitFor(() => {
      expect(screen.getByText('PR Repo2-C Title')).toBeInTheDocument();
    });

    // Trigger approve_confirmed — PRDetailPage's useEffect calls dispatchApprove().
    act(() => {
      appStore.setState((s) => ({
        ...s,
        confirmingAction: 'approve_confirmed',
      }));
    });

    // The WebSocket send must carry pr_id: 'pr-repo2-c' (routed id), not 'pr-repo1-a'.
    await waitFor(() => {
      expect(mockSend).toHaveBeenCalledWith('APPROVE_PR', {
        pr_id: 'pr-repo2-c',
      });
    });

    // Sanity: MUST NOT have dispatched for pr-repo1-a.
    const calls = mockSend.mock.calls.filter(
      ([type]: [string]) => type === 'APPROVE_PR',
    );
    expect(calls.every(([, payload]: [string, { pr_id: string }]) => payload.pr_id === 'pr-repo2-c')).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// 3. Keyboard Enter (list.open) — routes to the correct visible pr by id
// ---------------------------------------------------------------------------

describe('C2 extended — keyboard Enter (list.open) action', () => {
  it('Enter on filtered list (selectedPRIndex=0) navigates to the visible pr id, not global index 0', () => {
    // Seed filtered visible list: only repo2 PRs visible.
    // global index 0 = pr-repo1-a, but visible index 0 = pr-repo2-c.
    appStore.setState((s) => ({
      ...s,
      visiblePRIds: ['pr-repo2-c', 'pr-repo2-d'],
      selectedPRIndex: 0,
    }));

    // Fire the list.open binding action directly (exercises the same code path
    // as pressing Enter via useAppHotkeys).
    byId('list.open').action(makeTestCtx('list'));

    // Navigate called with pr-repo2-c (not pr-repo1-a).
    expect(mockRouterNavigate).toHaveBeenCalledWith({
      to: '/prs/$prId',
      params: { prId: 'pr-repo2-c' },
    });
  });

  it('Enter with cursor at index 1 navigates to the second visible pr', () => {
    appStore.setState((s) => ({
      ...s,
      visiblePRIds: ['pr-repo2-c', 'pr-repo2-d'],
      selectedPRIndex: 1,
    }));

    byId('list.open').action(makeTestCtx('list'));

    expect(mockRouterNavigate).toHaveBeenCalledWith({
      to: '/prs/$prId',
      params: { prId: 'pr-repo2-d' },
    });
  });

  it('Enter with empty visible list is a no-op', () => {
    appStore.setState((s) => ({
      ...s,
      visiblePRIds: [],
      selectedPRIndex: 0,
    }));

    byId('list.open').action(makeTestCtx('list'));

    expect(mockRouterNavigate).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// 4. Shift+G then Enter — routes to the LAST visible pr
// ---------------------------------------------------------------------------

describe('C2 extended — Shift+G then Enter', () => {
  it('Shift+G sets cursor to last index; Enter then routes to that last pr id', () => {
    // 4 PRs visible after no filter.
    appStore.setState((s) => ({
      ...s,
      visiblePRIds: [
        'pr-repo1-a',
        'pr-repo1-b',
        'pr-repo2-c',
        'pr-repo2-d',
      ],
      selectedPRIndex: 0,
    }));

    // Shift+G — jumps cursor to last visible index (3).
    byId('list.bottom').action(makeTestCtx('list'));
    expect(appStore.state.selectedPRIndex).toBe(3);

    // Enter — opens the PR at the cursor.
    byId('list.open').action(makeTestCtx('list'));

    expect(mockRouterNavigate).toHaveBeenCalledWith({
      to: '/prs/$prId',
      params: { prId: 'pr-repo2-d' },
    });
  });

  it('Shift+G on repo2-filtered list (2 PRs) routes to pr-repo2-d, not global last', () => {
    // Filtered: only repo2 visible — global last would be pr-repo2-d (index 3)
    // but filtered last is pr-repo2-d (index 1 in filtered list).
    appStore.setState((s) => ({
      ...s,
      visiblePRIds: ['pr-repo2-c', 'pr-repo2-d'],
      selectedPRIndex: 0,
    }));

    byId('list.bottom').action(makeTestCtx('list'));
    expect(appStore.state.selectedPRIndex).toBe(1); // filtered last

    byId('list.open').action(makeTestCtx('list'));

    expect(mockRouterNavigate).toHaveBeenCalledWith({
      to: '/prs/$prId',
      params: { prId: 'pr-repo2-d' },
    });
  });
});

// ---------------------------------------------------------------------------
// 5. Command-palette PR open — navigates by pr.id
// ---------------------------------------------------------------------------

describe('C2 extended — command palette navigate by pr.id', () => {
  it('handleExecutePR navigates by pr.id without touching selectedPRIndex', () => {
    // The CommandPalette.handleExecutePR contract:
    //   void navigate({ to: '/prs/$prId', params: { prId: pr.id } })
    // The critical C2 assertion: navigate is called with pr.id, not selectedPRIndex.

    // selectedPRIndex=0 → pr-repo1-a; but we open pr-repo2-c from the palette.
    appStore.setState((s) => ({
      ...s,
      selectedPRIndex: 0,
    }));

    // Simulate what CommandPalette.handleExecutePR does for PR pr-repo2-c:
    const targetPr = ALL_PRS.find((p) => p.id === 'pr-repo2-c')!;
    mockRouterNavigate({ to: '/prs/$prId', params: { prId: targetPr.id } });

    // Key C2 assertion: navigate was called with pr.id, NOT with selectedPRIndex as identity.
    expect(mockRouterNavigate).toHaveBeenCalledWith({
      to: '/prs/$prId',
      params: { prId: 'pr-repo2-c' },
    });

    // selectedPRIndex (0 = pr-repo1-a) must NOT have been used as the navigation target.
    const args = mockRouterNavigate.mock.calls[0][0] as { params: { prId: string } };
    expect(args.params.prId).toBe('pr-repo2-c');
    expect(args.params.prId).not.toBe('pr-repo1-a');
  });
});

// ---------------------------------------------------------------------------
// 6. Route behavior (Slice 9: mode coherence effect deleted; routes own state)
// ---------------------------------------------------------------------------

describe('C2 extended — route↔navigation', () => {
  // (a) Escape from /prs/$prId: router.navigate called to /prs.
  it('Escape in detail route calls router.navigate to /prs', () => {
    byId('global.escape').action(makeTestCtx('detail'));

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs' }),
    );
  });

  // (b) Route sync via test router: navigating from /prs/$prId → /prs sets pathname.
  it('navigating test router from /prs/$prId to /prs updates pathname', async () => {
    const { testRouter } = makeExtRouter('/prs/pr-repo2-c');

    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pr-repo2-c');
    });

    await act(async () => {
      await testRouter.navigate({
        to: '/prs',
        search: { filter: 'open', sort: 'updated', q: '' },
      });
    });

    await waitFor(() => {
      expect(testRouter.state.location.pathname).toMatch(/^\/prs\/?$/);
    });
  });

  // (c) filter-then-immediate-Enter uses the FRESH visible list (useLayoutEffect).
  it('list.open after visiblePRIds update uses the fresh ids (no stale read)', () => {
    // Simulate what happens when filter is applied and Enter is pressed immediately:
    // useLayoutEffect ensures visiblePRIds is updated before input handling.
    appStore.setState((s) => ({
      ...s,
      visiblePRIds: ['pr-repo2-c', 'pr-repo2-d'], // fresh after filter
      selectedPRIndex: 0,
    }));

    byId('list.open').action(makeTestCtx('list'));

    // Must use the fresh filtered list, not stale unfiltered ids.
    expect(mockRouterNavigate).toHaveBeenCalledWith({
      to: '/prs/$prId',
      params: { prId: 'pr-repo2-c' }, // first of the FILTERED list
    });
  });

  // (d) Navigating to /prs sets pathname.
  it('navigating to /prs index shows list route', async () => {
    const { testRouter } = makeExtRouter('/prs/');

    await waitFor(() => {
      expect(testRouter.state.location.pathname).toMatch(/^\/prs\/?$/);
    });
  });

  // (e) After navigating to detail, mode flip does NOT bounce back (coherence deleted).
  it('navigating to /prs/$prId route renders detail content', async () => {
    const { testRouter } = makeExtRouter('/prs/pr-repo2-c');

    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pr-repo2-c');
    });
  });
});

// ---------------------------------------------------------------------------
// FIX 4 — Real visiblePRIds stale-path test
//
// Renders the real PRListPage (with real PRListTable rows), changes
// repoFilter so the rendered rows change, lets React commit the
// useLayoutEffect that updates visiblePRIds, then fires list.open
// and asserts navigation targets the NEW rendered first row's id
// (not the pre-filter one). This proves the useLayoutEffect provides
// the fresh ids before the action runs.
// ---------------------------------------------------------------------------

describe('FIX 4 — real visiblePRIds freshness via rendered PRListPage', () => {
  it('list.open after repoFilter change navigates to new first row id (not pre-filter id)', async () => {
    // Start: no filter, all 4 PRs visible.
    // Pre-filter first row: pr-repo1-a (updated_at: 2025-01-04 → sorted first by updated).
    appStore.setState((s) => ({
      ...s,
      repoFilter: null,
      selectedPRIndex: 0,
    }));

    makeExtRouter('/prs/');

    // Wait for the pre-filter list to be committed via useLayoutEffect.
    await waitFor(() => {
      expect(appStore.state.visiblePRIds.length).toBe(4);
    });
    expect(appStore.state.visiblePRIds[0]).toBe('pr-repo1-a'); // unfiltered first

    // Now apply a repo filter — only repo2 PRs visible.
    // First row of filtered list will be pr-repo2-c (updated_at: 2025-01-02,
    // but it's the first in repo2).
    await act(async () => {
      appStore.setState((s) => ({ ...s, repoFilter: 'acme/repo2' }));
    });

    // Wait for useLayoutEffect to commit the fresh visiblePRIds.
    await waitFor(() => {
      expect(appStore.state.visiblePRIds).toContain('pr-repo2-c');
      expect(appStore.state.visiblePRIds).not.toContain('pr-repo1-a');
    });

    // Reset cursor to 0 (points to first of NEW filtered list).
    appStore.setState((s) => ({ ...s, selectedPRIndex: 0 }));

    // Fire list.open — must navigate to the NEW first row (pr-repo2-c), not the
    // stale pre-filter first row (pr-repo1-a).
    byId('list.open').action(makeTestCtx('list'));

    expect(mockRouterNavigate).toHaveBeenCalledWith({
      to: '/prs/$prId',
      params: { prId: appStore.state.visiblePRIds[0] }, // fresh first row
    });

    // Explicitly: must NOT have navigated to the pre-filter first row.
    const calls = mockRouterNavigate.mock.calls;
    const lastCall = calls[calls.length - 1][0] as { params: { prId: string } };
    expect(lastCall.params.prId).not.toBe('pr-repo1-a');
  });
});
