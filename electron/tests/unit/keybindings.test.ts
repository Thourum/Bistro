// @vitest-environment jsdom
import { Store } from '@tanstack/store';
import { assertValidHotkey } from '@tanstack/react-hotkeys';
import { describe, expect, it } from 'vitest';
import type { UIState } from '../../src/renderer/store/ui';
import type { KeyBinding } from '../../src/renderer/utils/keybindings';
import {
  DEFAULT_KEYBINDINGS,
  getEffectiveBindings,
  isSequenceBinding,
} from '../../src/renderer/utils/keybindings';
import { initialUIState } from '../../src/renderer/store/ui';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeStore(overrides: Partial<UIState> = {}): Store<UIState> {
  return new Store<UIState>({ ...initialUIState, ...overrides });
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

  it('every binding has keys, description, modes, and action', () => {
    for (const b of DEFAULT_KEYBINDINGS) {
      expect(typeof b.actionId).toBe('string');
      expect(b.actionId.length).toBeGreaterThan(0);
      expect(b.keys.length).toBeGreaterThanOrEqual(1);
      expect(typeof b.description).toBe('string');
      expect(b.description.length).toBeGreaterThan(0);
      expect(Array.isArray(b.modes)).toBe(true);
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
      expect(b.modes).toContain('list');
      expect(b.hidden ?? false).toBe(digit !== 1);
    }
  });

  it('includes quit and search chords with canonical modifiers', () => {
    expect(byId('global.quit.mac').keys).toBe('Meta+Q');
    expect(byId('global.quit.win').keys).toBe('Alt+F4');
    expect(byId('global.search.mac').keys).toBe('Meta+/');
    expect(byId('global.search.win').keys).toBe('Control+/');
  });

  it('covers the diff view: Space enters, Escape exits', () => {
    expect(byId('detail.viewDiff').keys).toBe('Space');
    expect(byId('detail.viewDiff').modes).toEqual(['detail']);
    expect(byId('global.escape').modes).toContain('diff');
  });

  it('? help binding is active in list and detail', () => {
    const b = byId('global.help');
    expect(b.keys).toBe('?');
    expect(b.modes).toContain('list');
    expect(b.modes).toContain('detail');
  });

  it('detail scroll bindings include 0 and $ jumps', () => {
    expect(byId('detail.scrollTop').keys).toBe('0');
    expect(byId('detail.scrollBottom').keys).toBe('$');
  });
});

// ---------------------------------------------------------------------------
// Action behavior (direct invocation)
// ---------------------------------------------------------------------------

describe('binding actions', () => {
  it('j moves to the next PR', () => {
    const store = makeStore({ selectedPRIndex: 2 });
    byId('list.down').action(store);
    expect(store.state.selectedPRIndex).toBe(3);
  });

  it('k moves to the previous PR and clamps at 0', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    byId('list.up').action(store);
    expect(store.state.selectedPRIndex).toBe(0);
  });

  it('gg jumps to the top', () => {
    const store = makeStore({ selectedPRIndex: 7 });
    byId('list.top').action(store);
    expect(store.state.selectedPRIndex).toBe(0);
  });

  it('Shift+G jumps to the bottom sentinel', () => {
    const store = makeStore();
    byId('list.bottom').action(store);
    expect(store.state.selectedPRIndex).toBe(Number.MAX_SAFE_INTEGER);
  });

  it('Enter opens the detail view', () => {
    const store = makeStore();
    byId('list.open').action(store);
    expect(store.state.mode).toBe('detail');
  });

  it('/ focuses search', () => {
    const store = makeStore();
    byId('list.search').action(store);
    expect(store.state.mode).toBe('search');
  });

  it('f toggles the filter menu', () => {
    const store = makeStore();
    byId('list.filter').action(store);
    expect(store.state.filterMenuOpen).toBe(true);
    expect(store.state.mode).toBe('filter');
    byId('list.filter').action(store);
    expect(store.state.filterMenuOpen).toBe(false);
    expect(store.state.mode).toBe('list');
  });

  it('digit bindings jump to the Nth PR', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    byId('list.jumpTo5').action(store);
    expect(store.state.selectedPRIndex).toBe(4);
  });

  it('aa requires a double tap to confirm approve', () => {
    const store = makeStore({ mode: 'detail' });
    byId('detail.approve').action(store);
    expect(store.state.confirmingAction).toBe('approve');
    byId('detail.approve').action(store);
    expect(store.state.confirmingAction).toBe('approve_confirmed');
  });

  it('Space in detail opens the diff view', () => {
    const store = makeStore({ mode: 'detail' });
    byId('detail.viewDiff').action(store);
    expect(store.state.mode).toBe('diff');
  });

  describe('Escape is context-sensitive', () => {
    const cases: Array<[UIState['mode'], UIState['mode']]> = [
      ['help', 'list'],
      ['diff', 'detail'],
      ['detail', 'list'],
      ['search', 'list'],
      ['settings', 'list'],
    ];
    for (const [from, to] of cases) {
      it(`${from} → ${to}`, () => {
        const store = makeStore({ mode: from });
        byId('global.escape').action(store);
        expect(store.state.mode).toBe(to);
      });
    }

    it('in list mode it closes the filter menu and cancels actions', () => {
      const store = makeStore({
        mode: 'list',
        filterMenuOpen: true,
        confirmingAction: 'approve',
      });
      byId('global.escape').action(store);
      expect(store.state.filterMenuOpen).toBe(false);
      expect(store.state.confirmingAction).toBeNull();
    });
  });

  describe('filter mode bindings', () => {
    const cases: Array<[string, UIState['filterState']]> = [
      ['filter.needsAction', 'needs_action'],
      ['filter.open', 'open'],
      ['filter.draft', 'draft'],
      ['filter.all', 'all'],
    ];
    for (const [actionId, expected] of cases) {
      it(`${actionId} sets filterState=${expected}`, () => {
        const store = makeStore({ mode: 'filter', filterMenuOpen: true });
        byId(actionId).action(store);
        expect(store.state.filterState).toBe(expected);
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
