// @vitest-environment jsdom
//
// /prs/$prId/diff route integration tests
//
// Covers:
//   1. Diff route renders the PR matching $prId (NOT selectedPRIndex).
//   2. Diff route passes the file matching ?file as selectedFileKey to DiffView.
//   3. Unknown ?file → falls back to first file (selectedFileKey='').
//   4. detail.viewDiff (Space) navigates to /prs/$prId/diff?file=....
//   5. Cold-start /prs/$prId/diff → mode:'diff'.
//   6. Navigate back to /prs/$prId → mode:'detail'.
//   7. Overlay close on diff route restores mode:'diff'.
//   8. In-diff n/p keybindings call router.navigate to update ?file.

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
import { act, render, screen, waitFor } from '@testing-library/react';
import React from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { z } from 'zod/v4';
import { AuthedRouteShell } from '../../src/renderer/components/AuthedRouteShell';
import { DiffPage } from '../../src/renderer/pages/DiffPage';
import type { RouterContext } from '../../src/renderer/router';
import { appStore } from '../../src/renderer/store';
import { initialUIState } from '../../src/renderer/store/ui';
import type { PR } from '../../src/renderer/types';
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

// Hoisted mock fns
const { mockRouterNavigate, mockRouterState, mockRequest } = vi.hoisted(() => {
  const mockRouterState = { location: { pathname: '/prs/pr-aaa/diff' } };
  return {
    mockRouterNavigate: vi.fn().mockResolvedValue(undefined),
    mockRouterState,
    mockRequest: vi.fn((type: string) => {
      if (type === 'REQUEST_COMMENTS') return Promise.resolve({ comments: [] });
      return Promise.resolve({ diff: '' });
    }),
  };
});

// WebSocket mock: request returns an empty diff by default.
vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    ws: null,
    send: vi.fn(),
    request: mockRequest,
  }),
}));

// Mock useReviewThreads so DiffPage's inline annotation path doesn't need a WS.
vi.mock('../../src/renderer/hooks/useReviewThreads', () => ({
  useReviewThreads: () => ({
    threads: [],
    loading: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

// ---------------------------------------------------------------------------
// Mock DiffView so we can inspect which selectedFileKey was passed.
// Render a testid marker so test assertions can query by file key.
// ---------------------------------------------------------------------------

const { mockDiffViewSelectedKey, mockDiffViewProps } = vi.hoisted(() => ({
  mockDiffViewSelectedKey: vi.fn<[string | undefined], null>(() => null),
  mockDiffViewProps: vi.fn<
    [{ threads?: unknown[]; comments?: unknown[] }],
    null
  >(() => null),
}));

vi.mock('../../src/renderer/components/diff/DiffView', () => ({
  DiffView: ({
    selectedFileKey,
    onFilesResolved,
    threads,
    comments,
  }: {
    diff: string;
    theme: string;
    findQuery?: string;
    selectedFileKey?: string;
    onFilesResolved?: (keys: string[]) => void;
    threads?: unknown[];
    comments?: unknown[];
  }) => {
    mockDiffViewSelectedKey(selectedFileKey);
    mockDiffViewProps({ threads, comments });
    // Simulate resolving 2 files so next/prev keybinding tests work.
    React.useEffect(() => {
      onFilesResolved?.(['src/alpha.ts', 'src/beta.ts']);
    }, [onFilesResolved]);
    return (
      <div
        data-testid="diff-view"
        data-selected-file-key={selectedFileKey ?? ''}
      >
        MockDiff
      </div>
    );
  },
}));

// ---------------------------------------------------------------------------
// Test PR data
// ---------------------------------------------------------------------------

const ALL_PRS: PR[] = [
  {
    id: 'pr-aaa',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'repo1',
    number: 101,
    title: 'PR AAA Title',
    author: 'alice',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-04T00:00:00Z',
    base_branch: 'main',
    head_branch: 'feat/aaa',
    additions: 10,
    deletions: 2,
    changed_files: 2,
    body: '',
    ci_status: 'success',
  },
  {
    id: 'pr-bbb',
    provider_id: 'github:default',
    owner: 'acme',
    repo: 'repo1',
    number: 102,
    title: 'PR BBB Title',
    author: 'bob',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-02T00:00:00Z',
    updated_at: '2025-01-03T00:00:00Z',
    base_branch: 'main',
    head_branch: 'feat/bbb',
    additions: 5,
    deletions: 1,
    changed_files: 1,
    body: '',
    ci_status: 'success',
  },
  {
    id: 'github:Itterno/Bistro:14',
    provider_id: 'github:default',
    owner: 'Itterno',
    repo: 'Bistro',
    number: 14,
    title: 'Router migration',
    author: 'alice',
    state: 'open',
    review_status: 'pending',
    created_at: '2025-01-05T00:00:00Z',
    updated_at: '2025-01-05T00:00:00Z',
    base_branch: 'main',
    head_branch: 'refactor/react-router-migration',
    additions: 1,
    deletions: 1,
    changed_files: 1,
    body: '',
    ci_status: 'success',
  },
];

vi.mock('../../src/renderer/hooks/useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({
    data: ALL_PRS,
    isLoading: false,
    error: null,
  }),
}));

vi.mock('../../src/renderer/router', () => ({
  router: {
    navigate: mockRouterNavigate,
    state: mockRouterState,
  },
  prDetailRoute: {},
  prDiffRoute: {},
}));

// ---------------------------------------------------------------------------
// Schemas and router factory
// ---------------------------------------------------------------------------

const diffSearchSchema = z.object({
  file: z.string().default(''),
  side: z.enum(['left', 'right']).default('right'),
});

function makeDiffRouteRouter(initialUrl = '/prs/pr-aaa/diff') {
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
    validateSearch: z.object({
      filter: z.enum(['open', 'draft', 'all', 'needs_action']).default('open'),
      sort: z.enum(['updated', 'created']).default('updated'),
      q: z.string().default(''),
    }),
    component: () => <div data-testid="pr-list-page">PRListPage</div>,
  });

  const prDetailRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '$prId',
    component: () => <div data-testid="pr-detail-page">DetailPage</div>,
  });

  const prDiffRoute = createRoute({
    getParentRoute: () => prsRoute,
    path: '$prId/diff',
    validateSearch: diffSearchSchema,
    component: DiffPage,
  });

  const routeTree = rootRoute.addChildren([
    prsRoute.addChildren([prsIndexRoute, prDetailRoute, prDiffRoute]),
  ]);

  const history = createMemoryHistory({ initialEntries: [initialUrl] });

  const testRouter = createRouter({
    routeTree,
    history,
    context: {
      queryClient,
    },
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
  store = appStore,
  routeBase:
    | import('../../src/renderer/utils/keybindings').RouteBase
    | null = 'detail',
  prId = 'pr-test-id',
): import('../../src/renderer/utils/keybindings').KeyActionContext {
  return {
    store,
    routeBase,
    activeOverlay: null,
    overlays: {
      activeOverlay: null,
      helpSourceRoute: 'list' as const,
      openCommand: vi.fn(),
      openHelp: vi.fn(),
      openSearch: vi.fn(),
      openFilter: vi.fn(),
      openThreads: vi.fn(),
      toggleFilter: vi.fn(),
      openRepoPicker: vi.fn(),
      closeOverlay: vi.fn(),
      closeSearch: vi.fn(),
    },
    location: { pathname: '/prs/pr-test-id' },
    prId,
  };
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

beforeEach(() => {
  mockRouterNavigate.mockClear();
  mockDiffViewSelectedKey.mockClear();
  mockDiffViewProps.mockClear();
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
// 1. Diff route renders PR by $prId (not selectedPRIndex)
// ---------------------------------------------------------------------------

describe('DiffPage resolves PR by id', () => {
  it('shows PR-101 header for pr-aaa even when selectedPRIndex=1 (pr-bbb)', async () => {
    // selectedPRIndex=1 points to pr-bbb (PR-102), but route is for pr-aaa (PR-101).
    appStore.setState((s) => ({ ...s, selectedPRIndex: 1 }));

    makeDiffRouteRouter('/prs/pr-aaa/diff?file=src%2Falpha.ts&side=right');

    await waitFor(() => {
      expect(screen.getByText('PR-101')).toBeInTheDocument();
    });

    // Must NOT show PR-102 (the index-based PR).
    expect(screen.queryByText('PR-102')).not.toBeInTheDocument();
  });

  it('shows not-found UI for unknown prId', async () => {
    // no-op (mode removed)
    makeDiffRouteRouter('/prs/pr-unknown-xyz/diff');

    await waitFor(() => {
      expect(screen.getByText('PR not found')).toBeInTheDocument();
    });
  });

  it('resolves URL-encoded provider PR ids without double-encoding', async () => {
    makeDiffRouteRouter('/prs/github%3AItterno%2FBistro%3A14/diff');

    await waitFor(() => {
      expect(screen.getByText('PR-14')).toBeInTheDocument();
    });

    expect(mockRequest).toHaveBeenCalledWith(
      'REQUEST_DIFF',
      expect.objectContaining({ pr_id: 'github:Itterno/Bistro:14' }),
    );
  });
});

// ---------------------------------------------------------------------------
// 2. ?file canonical resolution
// ---------------------------------------------------------------------------

describe('?file canonical resolution', () => {
  it('passes ?file key to DiffView as selectedFileKey', async () => {
    // no-op (mode removed)

    makeDiffRouteRouter('/prs/pr-aaa/diff?file=src%2Falpha.ts&side=right');

    await waitFor(() => {
      expect(screen.getByTestId('diff-view')).toBeInTheDocument();
    });

    const view = screen.getByTestId('diff-view');
    expect(view.getAttribute('data-selected-file-key')).toBe('src/alpha.ts');
  });

  it('passes empty selectedFileKey when ?file is absent (first-file fallback)', async () => {
    // no-op (mode removed)

    // No ?file param → defaults to '' per schema
    makeDiffRouteRouter('/prs/pr-aaa/diff');

    await waitFor(() => {
      expect(screen.getByTestId('diff-view')).toBeInTheDocument();
    });

    const view = screen.getByTestId('diff-view');
    // Empty string → DiffView falls back to first file internally.
    expect(view.getAttribute('data-selected-file-key')).toBe('');
  });

  it('passes ?file with spaces decoded correctly (path with space)', async () => {
    // no-op (mode removed)

    makeDiffRouteRouter('/prs/pr-aaa/diff?file=src%2Fmy%20file.ts&side=right');

    await waitFor(() => {
      expect(screen.getByTestId('diff-view')).toBeInTheDocument();
    });

    const view = screen.getByTestId('diff-view');
    expect(view.getAttribute('data-selected-file-key')).toBe('src/my file.ts');
  });
});

// ---------------------------------------------------------------------------
// 4. detail.viewDiff (Space) navigation
// ---------------------------------------------------------------------------

describe('detail.viewDiff navigation', () => {
  it('Space on /prs/$prId navigates to /prs/$prId/diff', () => {
    byId('detail.viewDiff').action(makeTestCtx(appStore, 'detail', 'pr-aaa'));

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/prs/$prId/diff',
        params: { prId: 'pr-aaa' },
      }),
    );
  });

  it('Space passes the clean provider PR id from route params', () => {
    byId('detail.viewDiff').action(
      makeTestCtx(appStore, 'detail', 'github:Itterno/Bistro:14'),
    );

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/prs/$prId/diff',
        params: { prId: 'github:Itterno/Bistro:14' },
      }),
    );
  });
});

// ---------------------------------------------------------------------------
// 5–7. Route behavior for diff route (routes own state; no mode coherence)
// ---------------------------------------------------------------------------

describe('diff route navigation', () => {
  it('cold-start /prs/$prId/diff renders at diff pathname', async () => {
    const { testRouter } = makeDiffRouteRouter('/prs/pr-aaa/diff');

    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pr-aaa/diff');
    });
  });

  it('navigate from diff to detail sets pathname to /prs/$prId', async () => {
    const { testRouter } = makeDiffRouteRouter('/prs/pr-aaa/diff');

    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pr-aaa/diff');
    });

    // Navigate back to detail route.
    act(() => {
      void testRouter.navigate({
        to: '/prs/$prId',
        params: { prId: 'pr-aaa' },
      });
    });

    await waitFor(() => {
      expect(testRouter.state.location.pathname).toBe('/prs/pr-aaa');
    });
  });

  it('diff route renders expected content', async () => {
    makeDiffRouteRouter('/prs/pr-aaa/diff');

    await waitFor(() => {
      expect(screen.queryByTestId('pr-not-found')).not.toBeInTheDocument();
    });
  });

  it('Escape returns to detail using the clean provider PR id from route params', () => {
    byId('global.escape').action(
      makeTestCtx(appStore, 'diff', 'github:Itterno/Bistro:14'),
    );

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/prs/$prId',
        params: { prId: 'github:Itterno/Bistro:14' },
      }),
    );
  });
});

// ---------------------------------------------------------------------------
// 8. In-diff next/prev updates ?file
// ---------------------------------------------------------------------------

describe('in-diff n/p keybindings update ?file', () => {
  beforeEach(() => {
    // Pre-populate diffFileKeys as if DiffPage already loaded the diff.
    appStore.setState((s) => ({
      ...s,
      diffFileKeys: ['src/alpha.ts', 'src/beta.ts'],
      selectedFileIndex: 0,
    }));
    // Ensure the mock router pathname is on the diff route.
    mockRouterState.location.pathname = '/prs/pr-aaa/diff';
  });

  it('n (nextFile) in diff mode calls navigate with next ?file key', () => {
    byId('detail.nextFile').action(makeTestCtx(appStore, 'diff', 'pr-aaa'));

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/prs/$prId/diff',
        params: { prId: 'pr-aaa' },
        search: expect.objectContaining({ file: 'src/beta.ts' }),
        replace: true,
      }),
    );
    expect(appStore.state.selectedFileIndex).toBe(1);
  });

  it('p (prevFile) at index 0 stays at index 0 and does not navigate past first file', () => {
    appStore.setState((s) => ({ ...s, selectedFileIndex: 0 }));
    byId('detail.prevFile').action(makeTestCtx(appStore, 'diff', 'pr-aaa'));

    // Navigate called with file='src/alpha.ts' (clamped to 0).
    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        search: expect.objectContaining({ file: 'src/alpha.ts' }),
      }),
    );
    expect(appStore.state.selectedFileIndex).toBe(0);
  });

  it('p (prevFile) at index 1 navigates back to first file', () => {
    appStore.setState((s) => ({ ...s, selectedFileIndex: 1 }));
    byId('detail.prevFile').action(makeTestCtx(appStore, 'diff', 'pr-aaa'));

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        search: expect.objectContaining({ file: 'src/alpha.ts' }),
      }),
    );
    expect(appStore.state.selectedFileIndex).toBe(0);
  });

  it('n/p in detail route does NOT call router.navigate', () => {
    byId('detail.nextFile').action(makeTestCtx(appStore, 'detail'));

    expect(mockRouterNavigate).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// Route-owned vs legacy mode coherence
// ---------------------------------------------------------------------------

// Route-owned coherence tests removed — mode field deleted; routes own state.
// Route ownership is now 100% router-controlled; there is no store mode
// to "correct". The underlying routing behavior (pathname) is tested above.

// ---------------------------------------------------------------------------
// DiffPage passes comments and threads to DiffView
// ---------------------------------------------------------------------------

describe('DiffPage inline annotations', () => {
  it('passes threads and comments props to DiffView', async () => {
    makeDiffRouteRouter('/prs/pr-aaa/diff');

    await waitFor(() => {
      expect(screen.getByTestId('diff-view')).toBeInTheDocument();
    });

    // DiffPage should pass threads and comments arrays (even if empty from mocks).
    expect(mockDiffViewProps).toHaveBeenCalledWith(
      expect.objectContaining({
        threads: expect.any(Array),
        comments: expect.any(Array),
      }),
    );
  });

  it('calls REQUEST_COMMENTS with the current pr_id', async () => {
    makeDiffRouteRouter('/prs/pr-aaa/diff');

    await waitFor(() => {
      expect(screen.getByTestId('diff-view')).toBeInTheDocument();
    });

    expect(mockRequest).toHaveBeenCalledWith(
      'REQUEST_COMMENTS',
      expect.objectContaining({ pr_id: 'pr-aaa' }),
    );
  });
});

// ---------------------------------------------------------------------------
// diffFileKeys lifecycle: no stale carryover across PRs
// ---------------------------------------------------------------------------

describe('diffFileKeys cleared on prId change', () => {
  it('DiffPage clears diffFileKeys when mounted with a new prId', async () => {
    // Seed stale keys from a previous PR's diff.
    appStore.setState((s) => ({
      ...s,
      diffFileKeys: ['old-pr/file-a.ts', 'old-pr/file-b.ts'],
    }));

    makeDiffRouteRouter('/prs/pr-bbb/diff');

    // After mounting, diffFileKeys should be cleared (then repopulated by the
    // mock DiffView via onFilesResolved).
    await waitFor(() => {
      // The mock DiffView calls onFilesResolved with ['src/alpha.ts','src/beta.ts'].
      // Verify stale keys are gone.
      expect(appStore.state.diffFileKeys).not.toContain('old-pr/file-a.ts');
    });
  });

  it('detail.viewDiff always navigates with file:empty (no stale key used)', () => {
    // Seed stale diffFileKeys from a previous PR.
    appStore.setState((s) => ({
      ...s,
      diffFileKeys: ['stale/key.ts'],
    }));
    byId('detail.viewDiff').action(makeTestCtx(appStore, 'detail', 'pr-bbb'));

    expect(mockRouterNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/prs/$prId/diff',
        params: { prId: 'pr-bbb' },
        search: expect.objectContaining({ file: '' }),
      }),
    );
  });
});
