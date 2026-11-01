// @vitest-environment jsdom

import { assertValidHotkey } from '@tanstack/react-hotkeys';
import { Store } from '@tanstack/store';
import { assert, describe, expect, it, vi } from 'vitest';
import type { OverlayContextValue } from '../../src/overlays/OverlayContext';
import type { UIState } from '../../src/store/ui';
import { initialUIState } from '../../src/store/ui';
import {
  getBindingAction,
  type KeyActionContext,
} from '../../src/utils/keybindingActions';
import type {
  KeyBinding,
  RouteBase,
} from '../../src/utils/keybindings';
import {
  ACTION_CATALOG,
  getEffectiveBindings,
  isActive,
  isSequenceBinding,
} from '../../src/utils/keybindings';
import { KEYMAPS } from '../../src/utils/keymaps';

// Mock the router so list.open / list.bottom can call router.navigate without
// a real RouterProvider.
const mockNavigate = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock('../../src/router', () => ({
  router: {
    navigate: mockNavigate,
    state: { location: { pathname: '/prs/pr-test-id' } },
  },
  prDetailRoute: {},
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeStore(overrides: Partial<UIState> = {}): Store<UIState> {
  return new Store<UIState>({ ...initialUIState, ...overrides });
}

/** Create a minimal KeyActionContext for testing binding actions. */
function makeCtx(
  storeOrOverrides: Store<UIState> | Partial<UIState> = {},
  routeBase: RouteBase | null = 'list',
  overlayOverrides: Partial<OverlayContextValue> = {},
): KeyActionContext {
  const store =
    storeOrOverrides instanceof Store
      ? storeOrOverrides
      : makeStore(storeOrOverrides);
  const overlays: OverlayContextValue = {
    activeOverlay: null,
    helpSourceRoute: 'list',
    openCommand: vi.fn(),
    openHelp: vi.fn(),
    openSearch: vi.fn(),
    openFilter: vi.fn(),
    toggleFilter: vi.fn(),
    openRepoPicker: vi.fn(),
    openAddRepo: vi.fn(),
    openLineComment: vi.fn(),
    openThreads: vi.fn(),
    closeOverlay: vi.fn(),
    closeSearch: vi.fn(),
    ...overlayOverrides,
  };
  return {
    store,
    routeBase,
    activeOverlay: overlays.activeOverlay,
    overlays,
    location: { pathname: '/prs' },
    prId: 'pr-test-id',
  };
}

function byId(actionId: string): KeyBinding {
  const binding = getEffectiveBindings('vim', {}).find(
    (candidate) => candidate.actionId === actionId,
  );
  assert(binding !== undefined, `no binding with actionId ${actionId}`);
  return binding;
}

function invokeAction(actionId: string, context: KeyActionContext): void {
  const action = getBindingAction(actionId);
  assert(action !== undefined, `no action for binding ${actionId}`);
  action(context);
}

// ---------------------------------------------------------------------------
// Definitions
// ---------------------------------------------------------------------------

describe('vim bindings', () => {
  it('exports at least 35 binding definitions', () => {
    expect(getEffectiveBindings('vim', {}).length).toBeGreaterThanOrEqual(35);
  });

  it('every configured binding resolves to a runtime action', () => {
    for (const binding of ACTION_CATALOG) {
      expect(getBindingAction(binding.actionId), binding.actionId).toBeTypeOf(
        'function',
      );
    }
  });

  it('actionIds are unique', () => {
    const ids = ACTION_CATALOG.map((b) => b.actionId);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('every key is valid TanStack Hotkeys syntax', () => {
    for (const b of getEffectiveBindings('vim', {})) {
      const steps = typeof b.keys === 'string' ? [b.keys] : b.keys;
      for (const step of steps) {
        expect(
          () => assertValidHotkey(step),
          `invalid hotkey "${step}" on ${b.actionId}`,
        ).not.toThrow();
      }
    }
  });

  it('includes the expected list-mode single keys', () => {
    const expected: Record<string, string> = {
      'list.down': 'j',
      'list.up': 'k',
      'list.bottom': 'Shift+G',
      'list.open': 'Enter',
      'list.search': '/',
      'list.filter': 'f',
      'list.openBrowser': 'o',
      'list.toggleGroup': 'Space',
      'list.draft': 'd',
      'list.mute': 'm',
      'list.star': 's',
      'list.pageDown': 'Control+D',
      'list.pageUp': 'Control+U',
    };
    for (const [actionId, keys] of Object.entries(expected)) {
      expect(byId(actionId).keys, actionId).toBe(keys);
    }
  });

  it('defines gg and aa as sequences', () => {
    expect(byId('list.top').keys).toEqual(['g', 'g']);
    expect(byId('detail.approve').keys).toEqual(['a', 'a']);
    expect(byId('detail.requestChanges').keys).toBe('Shift+R');
    expect(isSequenceBinding(byId('list.top'))).toBe(true);
    expect(isSequenceBinding(byId('list.down'))).toBe(false);
  });

  it('expands 1-9 into nine jump bindings with only the first visible', () => {
    for (let digit = 1; digit <= 9; digit++) {
      const b = byId(`list.jumpTo${digit}`);
      expect(b.keys).toBe(String(digit));
      expect(b.activation.routes).toContain('list');
      expect(b.hidden ?? false).toBe(digit !== 1);
    }
  });

  it('includes quit, search, and settings chords', () => {
    expect(byId('global.quit.mac').keys).toBe('Meta+Q');
    expect(byId('global.quit.win').keys).toBe('Alt+F4');
    // Single cross-platform binding — 'Mod' resolves per platform.
    expect(byId('global.search').keys).toBe('Mod+/');
    expect(byId('global.settings').keys).toBe('Mod+,');
  });

  it('writes shifted punctuation as Shift + physical key', () => {
    // The matcher requires exact modifier state, so '?' must be 'Shift+/'.
    expect(byId('global.help').keys).toBe('Shift+/');
    expect(byId('global.command').keys).toBe('Shift+;');
    expect(byId('detail.scrollBottom').keys).toBe('Shift+4');
  });

  it('provides hidden arrow-key aliases for navigation', () => {
    expect(byId('list.downArrow').keys).toBe('ArrowDown');
    expect(byId('list.upArrow').keys).toBe('ArrowUp');
    expect(byId('list.downArrow').hidden).toBe(true);
    expect(byId('detail.scrollDownArrow').keys).toBe('ArrowDown');
    expect(byId('detail.scrollLeftArrow').keys).toBe('ArrowLeft');
  });

  it('detail scrolling also works in the diff view', () => {
    for (const id of [
      'detail.scrollDown',
      'detail.scrollUp',
      'detail.pageDown',
      'detail.scrollTop',
      'detail.scrollBottom',
    ]) {
      expect(byId(id).activation.routes, id).toContain('diff');
    }
  });

  it('diff view has its own file nav (Shift+J/K) and comment nav (n/p), distinct from detail.nextFile/prevFile', () => {
    expect(byId('detail.nextFile').activation.routes).toEqual(['detail']);
    expect(byId('detail.prevFile').activation.routes).toEqual(['detail']);
    expect(byId('diff.nextFile').keys).toBe('Shift+J');
    expect(byId('diff.prevFile').keys).toBe('Shift+K');
    expect(byId('diff.nextFile').activation.routes).toEqual(['diff']);
    expect(byId('diff.prevFile').activation.routes).toEqual(['diff']);
    expect(byId('diff.nextComment').keys).toBe('n');
    expect(byId('diff.prevComment').keys).toBe('p');
    expect(byId('diff.nextComment').activation.routes).toEqual(['diff']);
    expect(byId('diff.prevComment').activation.routes).toEqual(['diff']);
  });

  it('covers the diff view: Space enters, Escape exits', () => {
    expect(byId('detail.viewDiff').keys).toBe('Space');
    expect(byId('detail.viewDiff').activation.routes).toEqual(['detail']);
    expect(byId('global.escape').activation.overlays).toContain('command');
    expect(byId('global.escape').activation.routes).toContain('diff');
  });

  it('adds since-last-review (s) and mark-viewed (v), scoped to diff route only', () => {
    const sinceReview = byId('diff.sinceLastReview');
    expect(sinceReview.keys).toBe('s');
    expect(sinceReview.activation.routes).toEqual(['diff']);

    const markViewed = byId('diff.markViewed');
    expect(markViewed.keys).toBe('v');
    expect(markViewed.activation.routes).toEqual(['diff']);

    // 's' is reused across routes: star in list, since-last-review in diff.
    expect(byId('list.star').activation.routes).toEqual(['list']);
  });

  it('keeps the since-last-review filter active across marking viewed and changing style', () => {
    const store = makeStore({ confirmingAction: null });
    const context = makeCtx(store, 'diff');
    invokeAction('diff.sinceLastReview', context);
    invokeAction('diff.markViewed', context);
    expect(store.state.confirmingAction).toBe('mark-viewed');
    expect(store.state.sinceLastReviewActive).toBe(true);
    store.setState((state) => ({ ...state, confirmingAction: null }));
    expect(store.state.sinceLastReviewActive).toBe(true);
    invokeAction('diff.toggleStyle', context);
    expect(store.state.confirmingAction).toBe('toggle-diff-style');
    expect(store.state.sinceLastReviewActive).toBe(true);
    invokeAction('diff.sinceLastReview', context);
    expect(store.state.sinceLastReviewActive).toBe(false);
    expect(store.state.confirmingAction).toBe('toggle-diff-style');
  });

  it('v sets the mark-viewed intent flag', () => {
    const store = makeStore({ confirmingAction: null });
    invokeAction('diff.markViewed', makeCtx(store, 'diff'));
    expect(store.state.confirmingAction).toBe('mark-viewed');
  });

  it('Shift+S sets a diff-style toggle intent without persisting directly', () => {
    const store = makeStore({ confirmingAction: null });
    invokeAction('diff.toggleStyle', makeCtx(store, 'diff'));
    expect(store.state.confirmingAction).toBe('toggle-diff-style');
  });

  it('help binding is active in list and detail and records its source', () => {
    const b = byId('global.help');
    expect(b.activation.routes).toContain('list');
    expect(b.activation.routes).toContain('detail');
    const openHelpMock = vi.fn();
    const ctx = makeCtx(makeStore(), 'detail', { openHelp: openHelpMock });
    invokeAction(b.actionId, ctx);
    expect(openHelpMock).toHaveBeenCalledWith('detail');
  });

  // Regression: a Radix dialog dismisses Escape from a capture-phase listener,
  // so by the time the bubble-phase binding runs the overlay is already closed.
  // The binding is handed the overlay state from keypress time, and must close
  // the overlay WITHOUT also running the route branch — dismissing the help
  // overlay on /diff previously navigated out of the diff too.
  it('escape with an overlay open closes it and does not navigate', () => {
    mockNavigate.mockClear();
    const closeOverlay = vi.fn();
    invokeAction(
      'global.escape',
      makeCtx(makeStore(), 'diff', { activeOverlay: 'help', closeOverlay }),
    );
    expect(closeOverlay).toHaveBeenCalledTimes(1);
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('escape on the diff route with no overlay navigates back to the PR', () => {
    mockNavigate.mockClear();
    invokeAction('global.escape', makeCtx(makeStore(), 'diff'));
    expect(mockNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs/$prId' }),
    );
  });

  // `z` resolves the focused file by indexing diffFileKeys, so it can address
  // the wrong file or write a bogus key — the only new diff binding whose
  // action is more than a flag flip.
  it('z toggles the focused file in and out of the collapse overrides', () => {
    const store = makeStore({
      diffFileKeys: ['first.ts', 'second.ts'],
      selectedFileIndex: 1,
      diffCollapseOverrides: [],
    });
    invokeAction('diff.toggleCollapse', makeCtx(store, 'diff'));
    expect(store.state.diffCollapseOverrides).toEqual(['second.ts']);
    invokeAction('diff.toggleCollapse', makeCtx(store, 'diff'));
    expect(store.state.diffCollapseOverrides).toEqual([]);
  });

  it('z is a no-op when no file key is resolved', () => {
    const store = makeStore({
      diffFileKeys: [],
      selectedFileIndex: 0,
      diffCollapseOverrides: [],
    });
    invokeAction('diff.toggleCollapse', makeCtx(store, 'diff'));
    expect(store.state.diffCollapseOverrides).toEqual([]);
  });

  it('detail scroll bindings include top and bottom jumps', () => {
    expect(byId('detail.scrollTop').keys).toBe('0');
  });
});

// ---------------------------------------------------------------------------
// getEffectiveBindings — settings overrides
// ---------------------------------------------------------------------------

describe('getEffectiveBindings', () => {
  it('returns defaults when no overrides exist', () => {
    const bindings = getEffectiveBindings('vim', {});
    expect(bindings.map((binding) => binding.actionId).sort()).toEqual(
      Object.keys(KEYMAPS.vim).sort(),
    );
    expect(bindings.find((b) => b.actionId === 'list.down')?.keys).toBe('j');
  });

  it('an empty array removes the binding', () => {
    const bindings = getEffectiveBindings('vim', { 'list.down': [] });
    expect(bindings.find((b) => b.actionId === 'list.down')).toBeUndefined();
  });

  it('a one-element array replaces with a single hotkey', () => {
    const bindings = getEffectiveBindings('vim', { 'list.down': ['x'] });
    expect(bindings.find((b) => b.actionId === 'list.down')?.keys).toBe('x');
  });

  it('a multi-element array replaces with a sequence', () => {
    const bindings = getEffectiveBindings('vim', { 'list.down': ['x', 'x'] });
    const b = bindings.find((bb) => bb.actionId === 'list.down');
    expect(b?.keys).toEqual(['x', 'x']);
    expect(b !== undefined && isSequenceBinding(b)).toBe(true);
  });

  it('does not mutate the defaults', () => {
    getEffectiveBindings('vim', { 'list.down': ['x'] });
    expect(byId('list.down').keys).toBe('j');
  });
});

// ---------------------------------------------------------------------------
// BLOCKING 2 — login bindings must be inactive on every authed route/overlay
// ---------------------------------------------------------------------------

describe('isActive — login bindings inactive on authed routes', () => {
  // All login.provider.* bindings use activation: { routes: ["login"] }
  const loginBinding = getEffectiveBindings('vim', {}).find((b) =>
    b.actionId.startsWith('login.provider.'),
  );

  it('login binding exists in the resolved vim bindings', () => {
    expect(loginBinding).toBeDefined();
    expect(loginBinding?.activation.routes).toEqual(['login']);
  });

  const routes: Array<RouteBase | null> = [
    'list',
    'detail',
    'diff',
    'settings',
    null,
  ];
  const overlays = [
    null,
    'command',
    'help',
    'search',
    'filter',
    'repoPicker',
  ] as const;

  for (const routeBase of routes) {
    for (const activeOverlay of overlays) {
      it(`inactive: routeBase=${String(routeBase)} overlay=${String(activeOverlay)}`, () => {
        if (!loginBinding) return;
        expect(isActive(loginBinding, { routeBase, activeOverlay })).toBe(
          false,
        );
      });
    }
  }
});

// ---------------------------------------------------------------------------
// NIT 3 — global.quit fires from all app routes/overlays
// ---------------------------------------------------------------------------

describe('isActive — global.quit fires on routes and overlays', () => {
  const quitMac = byId('global.quit.mac');

  it('active on list route, no overlay', () => {
    expect(isActive(quitMac, { routeBase: 'list', activeOverlay: null })).toBe(
      true,
    );
  });
  it('active while help overlay is open', () => {
    expect(isActive(quitMac, { routeBase: null, activeOverlay: 'help' })).toBe(
      true,
    );
  });
  it('active while search overlay is open', () => {
    expect(
      isActive(quitMac, { routeBase: null, activeOverlay: 'search' }),
    ).toBe(true);
  });
  it('active while command overlay is open', () => {
    expect(
      isActive(quitMac, { routeBase: null, activeOverlay: 'command' }),
    ).toBe(true);
  });
  it('active while repo picker overlay is open', () => {
    expect(
      isActive(quitMac, { routeBase: null, activeOverlay: 'repoPicker' }),
    ).toBe(true);
  });
  it('active on login and onboarding routes', () => {
    expect(isActive(quitMac, { routeBase: 'login', activeOverlay: null })).toBe(
      true,
    );
    expect(
      isActive(quitMac, { routeBase: 'onboarding', activeOverlay: null }),
    ).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// isActive semantics — empty routes array
// ---------------------------------------------------------------------------

describe('isActive — empty routes array means never-active', () => {
  const neverBinding: KeyBinding = {
    actionId: 'test.never',
    keys: 'x',
    description: 'never',
    activation: { routes: [] },
  };

  it('inactive on list', () =>
    expect(
      isActive(neverBinding, { routeBase: 'list', activeOverlay: null }),
    ).toBe(false));
  it('inactive on detail', () =>
    expect(
      isActive(neverBinding, { routeBase: 'detail', activeOverlay: null }),
    ).toBe(false));
  it('inactive with overlay', () =>
    expect(
      isActive(neverBinding, { routeBase: 'list', activeOverlay: 'command' }),
    ).toBe(false));
  it('inactive with null routeBase', () =>
    expect(
      isActive(neverBinding, { routeBase: null, activeOverlay: null }),
    ).toBe(false));
});

// ---------------------------------------------------------------------------
// Phase 2.4 — review-threads bindings
// ---------------------------------------------------------------------------

describe('review-threads bindings', () => {
  it('gu (detail.threads) is a sequence binding scoped to the detail route', () => {
    const binding = byId('detail.threads');
    expect(binding.keys).toEqual(['g', 'u']);
    expect(binding.activation.routes).toEqual(['detail']);
  });

  it('gu opens the threads overlay', () => {
    const openThreadsMock = vi.fn();
    const ctx = makeCtx(makeStore(), 'detail', {
      openThreads: openThreadsMock,
    });
    invokeAction('detail.threads', ctx);
    expect(openThreadsMock).toHaveBeenCalled();
  });

  it('Escape closes the threads overlay', () => {
    const closeOverlayMock = vi.fn();
    const ctx = makeCtx(makeStore(), 'detail', {
      closeOverlay: closeOverlayMock,
      activeOverlay: 'threads',
    });
    invokeAction('global.escape', ctx);
    expect(closeOverlayMock).toHaveBeenCalled();
  });

  it('threads navigation/action keys are scoped to threads overlay only', () => {
    for (const id of [
      'threads.next',
      'threads.prev',
      'threads.resolve',
      'threads.unresolve',
    ]) {
      expect(byId(id).activation.overlays).toEqual(['threads']);
    }
  });

  it('threads keys dispatch their matching DOM custom events', () => {
    const cases: Array<[string, string]> = [
      ['threads.next', 'bistro:threads:next'],
      ['threads.prev', 'bistro:threads:prev'],
      ['threads.resolve', 'bistro:threads:resolve'],
      ['threads.unresolve', 'bistro:threads:unresolve'],
    ];
    for (const [id, eventName] of cases) {
      const seen: string[] = [];
      const listener = (): void => {
        seen.push(eventName);
      };
      document.addEventListener(eventName, listener);
      invokeAction(id, makeCtx(makeStore(), null));
      document.removeEventListener(eventName, listener);
      expect(seen).toEqual([eventName]);
    }
  });
});
