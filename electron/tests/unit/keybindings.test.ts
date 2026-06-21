// @vitest-environment jsdom

import { assertValidHotkey } from '@tanstack/react-hotkeys';
import { Store } from '@tanstack/store';
import { describe, expect, it, vi } from 'vitest';
import type { OverlayContextValue } from '../../src/renderer/overlays/OverlayContext';
import type { UIState } from '../../src/renderer/store/ui';
import { initialUIState } from '../../src/renderer/store/ui';
import type {
  KeyActionContext,
  KeyBinding,
  RouteBase,
} from '../../src/renderer/utils/keybindings';
import {
  DEFAULT_KEYBINDINGS,
  getEffectiveBindings,
  isActive,
  isSequenceBinding,
} from '../../src/renderer/utils/keybindings';

// Mock the router so list.open / list.bottom can call router.navigate without
// a real RouterProvider.
const mockNavigate = vi.hoisted(() => vi.fn().mockResolvedValue(undefined));
vi.mock('../../src/renderer/router', () => ({
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
  };
}

function byId(actionId: string): KeyBinding {
  const binding = DEFAULT_KEYBINDINGS.find((b) => b.actionId === actionId);
  if (binding === undefined) {
    throw new Error(`no binding with actionId ${actionId}`);
  }
  return binding;
}

// ---------------------------------------------------------------------------
// Definitions
// ---------------------------------------------------------------------------

describe('DEFAULT_KEYBINDINGS', () => {
  it('exports at least 35 binding definitions', () => {
    expect(DEFAULT_KEYBINDINGS.length).toBeGreaterThanOrEqual(35);
  });

  it('every binding has keys, description, activation, and action', () => {
    for (const b of DEFAULT_KEYBINDINGS) {
      expect(typeof b.actionId).toBe('string');
      expect(b.actionId.length).toBeGreaterThan(0);
      expect(b.keys.length).toBeGreaterThanOrEqual(1);
      expect(typeof b.description).toBe('string');
      expect(b.description.length).toBeGreaterThan(0);
      expect(typeof b.activation).toBe('object');
      expect(typeof b.action).toBe('function');
    }
  });

  it('actionIds are unique', () => {
    const ids = DEFAULT_KEYBINDINGS.map((b) => b.actionId);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('every key is valid TanStack Hotkeys syntax', () => {
    for (const b of DEFAULT_KEYBINDINGS) {
      const steps = isSequenceBinding(b) ? b.keys : [b.keys];
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
      'list.preview': 'Space',
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

  it('detail navigation and scrolling also work in the diff view', () => {
    for (const id of [
      'detail.nextFile',
      'detail.prevFile',
      'detail.scrollDown',
      'detail.scrollUp',
      'detail.pageDown',
      'detail.scrollTop',
      'detail.scrollBottom',
    ]) {
      expect(byId(id).activation.routes, id).toContain('diff');
    }
  });

  it('covers the diff view: Space enters, Escape exits', () => {
    expect(byId('detail.viewDiff').keys).toBe('Space');
    expect(byId('detail.viewDiff').activation.routes).toEqual(['detail']);
    expect(byId('global.escape').activation.overlays).toContain('command');
    expect(byId('global.escape').activation.routes).toContain('diff');
  });

  it('help binding is active in list and detail routes', () => {
    const b = byId('global.help');
    expect(b.activation.routes).toContain('list');
    expect(b.activation.routes).toContain('detail');
    const openHelpMock = vi.fn();
    const ctx = makeCtx(makeStore(), 'detail', { openHelp: openHelpMock });
    b.action(ctx);
    expect(openHelpMock).toHaveBeenCalledWith('detail');
  });

  it('detail scroll bindings include top and bottom jumps', () => {
    expect(byId('detail.scrollTop').keys).toBe('0');
  });
});

// ---------------------------------------------------------------------------
// Action behavior (direct invocation)
// ---------------------------------------------------------------------------

describe('binding actions', () => {
  it('j moves to the next PR', () => {
    const store = makeStore({ selectedPRIndex: 2 });
    byId('list.down').action(makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(3);
  });

  it('k moves to the previous PR and clamps at 0', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    byId('list.up').action(makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(0);
  });

  it('gg jumps to the top', () => {
    const store = makeStore({ selectedPRIndex: 7 });
    byId('list.top').action(makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(0);
  });

  it('Shift+G jumps to the last visible PR (not MAX_SAFE_INTEGER)', () => {
    // With 3 visible PRs, cursor should jump to index 2.
    const store = makeStore({ visiblePRIds: ['a', 'b', 'c'] });
    byId('list.bottom').action(makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(2);
    // With empty list, clamps to 0.
    const emptyStore = makeStore({ visiblePRIds: [] });
    byId('list.bottom').action(makeCtx(emptyStore));
    expect(emptyStore.state.selectedPRIndex).toBe(0);
  });

  it('Enter navigates to the PR at the cursor position by id', () => {
    mockNavigate.mockClear();
    // With a visible PR at index 0, Enter should call router.navigate.
    const store = makeStore({ visiblePRIds: ['pr-abc'], selectedPRIndex: 0 });
    byId('list.open').action(makeCtx(store));
    expect(mockNavigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs/$prId', params: { prId: 'pr-abc' } }),
    );
    // When list is empty, Enter is a no-op.
    mockNavigate.mockClear();
    const emptyStore = makeStore({ visiblePRIds: [], selectedPRIndex: 0 });
    byId('list.open').action(makeCtx(emptyStore));
    expect(mockNavigate).not.toHaveBeenCalled();
  });

  it('/ opens search overlay', () => {
    const openSearch = vi.fn();
    byId('list.search').action(makeCtx({}, 'list', { openSearch }));
    expect(openSearch).toHaveBeenCalled();
  });

  it('f toggles the filter overlay', () => {
    const toggleFilter = vi.fn();
    byId('list.filter').action(makeCtx({}, 'list', { toggleFilter }));
    expect(toggleFilter).toHaveBeenCalled();
  });

  it('digit bindings jump to the Nth PR', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    byId('list.jumpTo5').action(makeCtx(store));
    expect(store.state.selectedPRIndex).toBe(4);
  });

  it('aa requires a double tap to confirm approve', () => {
    const store = makeStore({});
    byId('detail.approve').action(makeCtx(store, 'detail'));
    expect(store.state.confirmingAction).toBe('approve');
    byId('detail.approve').action(makeCtx(store, 'detail'));
    expect(store.state.confirmingAction).toBe('approve_confirmed');
  });

  it('Space in detail opens the diff view (navigates to /prs/$prId/diff)', async () => {
    const store = makeStore({});
    const { router } = await import('../../src/renderer/router');
    (router.state.location as { pathname: string }).pathname = '/prs/pr-test-id';
    byId('detail.viewDiff').action(makeCtx(store, 'detail'));
    // Router navigates; mode is no longer written.
    expect(router.navigate).toHaveBeenCalledWith(
      expect.objectContaining({ to: '/prs/$prId/diff' }),
    );
  });

  describe('Escape is context-sensitive', () => {
    it('closes overlay when command overlay is active', () => {
      const closeOverlay = vi.fn();
      const ctx = makeCtx({}, 'list', {
        closeOverlay,
        activeOverlay: 'command',
      });
      // activeOverlay must match ctx.activeOverlay too
      Object.assign(ctx, { activeOverlay: 'command' });
      byId('global.escape').action(ctx);
      expect(closeOverlay).toHaveBeenCalled();
    });

    it('closes search overlay and clears query on Escape', () => {
      const closeSearch = vi.fn();
      const ctx = makeCtx({}, 'list', { closeSearch });
      Object.assign(ctx, { activeOverlay: 'search' });
      byId('global.escape').action(ctx);
      expect(closeSearch).toHaveBeenCalledWith({ clearQuery: true });
    });

    it('detail route → navigates to /prs (router.navigate called)', async () => {
      const { router } = await import('../../src/renderer/router');
      (router.state.location as { pathname: string }).pathname = '/prs/pr-test-id';
      const store = makeStore({});
      byId('global.escape').action(makeCtx(store, 'detail'));
      expect(router.navigate).toHaveBeenCalledWith(
        expect.objectContaining({ to: '/prs' }),
      );
    });

    it('diff route → navigates back to detail (router.navigate called)', async () => {
      const { router } = await import('../../src/renderer/router');
      (router.state.location as { pathname: string }).pathname = '/prs/pr-test-id/diff';
      const store = makeStore({});
      byId('global.escape').action(makeCtx(store, 'diff'));
      expect(router.navigate).toHaveBeenCalledWith(
        expect.objectContaining({ to: '/prs/$prId' }),
      );
    });

    it('settings route → navigates to /prs (router.navigate called)', async () => {
      const { router } = await import('../../src/renderer/router');
      (router.state.location as { pathname: string }).pathname = '/settings';
      const store = makeStore({});
      byId('global.escape').action(makeCtx(store, 'settings'));
      expect(router.navigate).toHaveBeenCalledWith(
        expect.objectContaining({ to: '/prs' }),
      );
    });

    it('in list mode it cancels confirming actions', () => {
      const store = makeStore({
        confirmingAction: 'approve',
      });
      byId('global.escape').action(makeCtx(store, 'list'));
      expect(store.state.confirmingAction).toBeNull();
    });
  });

  describe('filter overlay bindings', () => {
    const cases: Array<[string, UIState['filterState']]> = [
      ['filter.needsAction', 'needs_action'],
      ['filter.open', 'open'],
      ['filter.draft', 'draft'],
      ['filter.all', 'all'],
    ];
    for (const [actionId, expected] of cases) {
      it(`${actionId} sets filterState=${expected} and closes overlay`, () => {
        const closeOverlay = vi.fn();
        const store = makeStore();
        byId(actionId).action(makeCtx(store, 'list', {
          closeOverlay,
          activeOverlay: 'filter' as const,
        }));
        expect(store.state.filterState).toBe(expected);
        expect(closeOverlay).toHaveBeenCalled();
      });
    }
  });
});

// ---------------------------------------------------------------------------
// getEffectiveBindings — settings overrides
// ---------------------------------------------------------------------------

describe('getEffectiveBindings', () => {
  it('returns defaults when no overrides exist', () => {
    const bindings = getEffectiveBindings({});
    expect(bindings).toHaveLength(DEFAULT_KEYBINDINGS.length);
    expect(bindings.find((b) => b.actionId === 'list.down')?.keys).toBe('j');
  });

  it('an empty array removes the binding', () => {
    const bindings = getEffectiveBindings({ 'list.down': [] });
    expect(bindings.find((b) => b.actionId === 'list.down')).toBeUndefined();
  });

  it('a one-element array replaces with a single hotkey', () => {
    const bindings = getEffectiveBindings({ 'list.down': ['x'] });
    expect(bindings.find((b) => b.actionId === 'list.down')?.keys).toBe('x');
  });

  it('a multi-element array replaces with a sequence', () => {
    const bindings = getEffectiveBindings({ 'list.down': ['x', 'x'] });
    const b = bindings.find((bb) => bb.actionId === 'list.down');
    expect(b?.keys).toEqual(['x', 'x']);
    expect(b !== undefined && isSequenceBinding(b)).toBe(true);
  });

  it('does not mutate the defaults', () => {
    getEffectiveBindings({ 'list.down': ['x'] });
    expect(byId('list.down').keys).toBe('j');
  });
});

// ---------------------------------------------------------------------------
// BLOCKING 2 — login bindings must be inactive on every authed route/overlay
// ---------------------------------------------------------------------------

describe('isActive — login bindings inactive on authed routes', () => {
  // All login.provider.* bindings use activation: { routes: ["login"] }
  const loginBinding = DEFAULT_KEYBINDINGS.find((b) =>
    b.actionId.startsWith('login.provider.'),
  );

  it('login binding exists in DEFAULT_KEYBINDINGS', () => {
    expect(loginBinding).toBeDefined();
    expect(loginBinding?.activation.routes).toEqual(['login']);
  });

  const routes: Array<RouteBase | null> = ['list', 'detail', 'diff', 'settings', null];
  const overlays = [null, 'command', 'help', 'search', 'filter', 'repoPicker'] as const;

  for (const routeBase of routes) {
    for (const activeOverlay of overlays) {
      it(`inactive: routeBase=${String(routeBase)} overlay=${String(activeOverlay)}`, () => {
        if (!loginBinding) return;
        expect(
          isActive(loginBinding, { routeBase, activeOverlay }),
        ).toBe(false);
      });
    }
  }
});

// ---------------------------------------------------------------------------
// NIT 3 — global.quit fires while help/search overlays are open
// ---------------------------------------------------------------------------

describe('isActive — global.quit fires on routes AND help/search overlays', () => {
  const quitMac = DEFAULT_KEYBINDINGS.find((b) => b.actionId === 'global.quit.mac')!;

  it('active on list route, no overlay', () => {
    expect(isActive(quitMac, { routeBase: 'list', activeOverlay: null })).toBe(true);
  });
  it('active while help overlay is open', () => {
    expect(isActive(quitMac, { routeBase: null, activeOverlay: 'help' })).toBe(true);
  });
  it('active while search overlay is open', () => {
    expect(isActive(quitMac, { routeBase: null, activeOverlay: 'search' })).toBe(true);
  });
  it('inactive while command overlay is open', () => {
    // command was NOT in old quit modes — keep it inactive.
    expect(isActive(quitMac, { routeBase: null, activeOverlay: 'command' })).toBe(false);
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
    action: () => undefined,
  };

  it('inactive on list', () => expect(isActive(neverBinding, { routeBase: 'list', activeOverlay: null })).toBe(false));
  it('inactive on detail', () => expect(isActive(neverBinding, { routeBase: 'detail', activeOverlay: null })).toBe(false));
  it('inactive with overlay', () => expect(isActive(neverBinding, { routeBase: 'list', activeOverlay: 'command' })).toBe(false));
  it('inactive with null routeBase', () => expect(isActive(neverBinding, { routeBase: null, activeOverlay: null })).toBe(false));
});
