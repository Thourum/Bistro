// @vitest-environment jsdom
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { Store } from '@tanstack/store';
import type { UIState } from '../../src/renderer/store/ui';
import { KEYBOARD_BINDING_GROUPS, bindingSequence } from '../../src/renderer/utils/keyboard';
import type { BindingDefinition } from '../../src/renderer/utils/keyboard';
import { resolveEventKey, findBinding as findBindingFn } from '../../src/renderer/hooks/useKeyboard';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeStore(overrides: Partial<UIState> = {}): Store<UIState> {
  const initial: UIState = {
    selectedPRIndex: 0,
    selectedFileIndex: 0,
    searchQuery: '',
    mode: 'list',
    paneLayout: 'split',
    sortBy: 'updated',
    filterState: 'open',
    confirmingAction: null,
    filterMenuOpen: false,
    ...overrides,
  };
  return new Store<UIState>(initial);
}

function findBinding(keys: string[]): BindingDefinition | undefined {
  const seq = keys.join('');
  return KEYBOARD_BINDING_GROUPS.find((b) => bindingSequence(b) === seq);
}

// ---------------------------------------------------------------------------
// KEYBOARD_BINDING_GROUPS — utility / definitions
// ---------------------------------------------------------------------------

describe('KEYBOARD_BINDING_GROUPS', () => {
  it('exports at least 25 binding definitions', () => {
    expect(KEYBOARD_BINDING_GROUPS.length).toBeGreaterThanOrEqual(25);
  });

  it('every binding has keys, description, modes, and action', () => {
    for (const b of KEYBOARD_BINDING_GROUPS) {
      expect(Array.isArray(b.keys)).toBe(true);
      expect(b.keys.length).toBeGreaterThanOrEqual(1);
      expect(typeof b.description).toBe('string');
      expect(b.description.length).toBeGreaterThan(0);
      expect(Array.isArray(b.modes)).toBe(true);
      expect(typeof b.action).toBe('function');
    }
  });

  it('includes all expected single-key list bindings', () => {
    const expectedKeys = ['j', 'k', 'G', 'Enter', '/', 'f', 'o', 'd', 'm', 's', ' ', ':'];
    for (const key of expectedKeys) {
      const b = findBinding([key]);
      expect(b, `expected binding for key "${JSON.stringify(key)}"`).toBeDefined();
    }
  });

  it('includes ctrl+d and ctrl+u page-scroll bindings', () => {
    expect(findBinding(['ctrl+d'])).toBeDefined();
    expect(findBinding(['ctrl+u'])).toBeDefined();
  });

  it('includes meta+q (quit macOS) and alt+F4 (quit Linux/Windows)', () => {
    expect(findBinding(['meta+q'])).toBeDefined();
    expect(findBinding(['alt+F4'])).toBeDefined();
  });

  it('includes gg (two-key sequence)', () => {
    const b = findBinding(['g', 'g']);
    expect(b).toBeDefined();
    expect(b!.keys).toEqual(['g', 'g']);
  });

  it('includes aa (two-key sequence for approve)', () => {
    const b = findBinding(['a', 'a']);
    expect(b).toBeDefined();
    expect(b!.keys).toEqual(['a', 'a']);
  });

  it('includes ? for help', () => {
    const b = findBinding(['?']);
    expect(b).toBeDefined();
    expect(b!.modes).toContain('list');
    expect(b!.modes).toContain('detail');
  });

  it('includes Escape binding covering all modes', () => {
    const b = findBinding(['Escape']);
    expect(b).toBeDefined();
    expect(b!.modes).toContain('list');
    expect(b!.modes).toContain('detail');
    expect(b!.modes).toContain('help');
    expect(b!.modes).toContain('search');
  });

  it('has no help-overlay stub bindings (deferred to HelpOverlay task)', () => {
    const helpOnlyBindings = KEYBOARD_BINDING_GROUPS.filter(
      (b) => b.modes.length === 1 && b.modes[0] === 'help'
    );
    expect(helpOnlyBindings).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// bindingSequence utility
// ---------------------------------------------------------------------------

describe('bindingSequence', () => {
  it('returns the single key for single-key bindings', () => {
    expect(bindingSequence({ keys: ['j'], description: '', modes: [], action: () => {} })).toBe('j');
  });

  it('joins keys for multi-key bindings', () => {
    expect(bindingSequence({ keys: ['g', 'g'], description: '', modes: [], action: () => {} })).toBe('gg');
  });
});

// ---------------------------------------------------------------------------
// Simple key action tests (direct binding invocation)
// ---------------------------------------------------------------------------

describe('j binding — move to next PR', () => {
  it('increments selectedPRIndex by 1', () => {
    const store = makeStore({ selectedPRIndex: 3 });
    const b = findBinding(['j'])!;
    b.action(store);
    expect(store.state.selectedPRIndex).toBe(4);
  });
});

describe('k binding — move to previous PR', () => {
  it('decrements selectedPRIndex by 1', () => {
    const store = makeStore({ selectedPRIndex: 3 });
    const b = findBinding(['k'])!;
    b.action(store);
    expect(store.state.selectedPRIndex).toBe(2);
  });

  it('does not go below 0', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    const b = findBinding(['k'])!;
    b.action(store);
    expect(store.state.selectedPRIndex).toBe(0);
  });
});

describe('Enter binding — open selected PR', () => {
  it('switches mode to detail', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['Enter'])!;
    b.action(store);
    expect(store.state.mode).toBe('detail');
  });
});

describe('Escape binding — context-sensitive', () => {
  it('returns from help to list', () => {
    const store = makeStore({ mode: 'help' });
    const b = findBinding(['Escape'])!;
    b.action(store);
    expect(store.state.mode).toBe('list');
  });

  it('returns from detail to list', () => {
    const store = makeStore({ mode: 'detail' });
    const b = findBinding(['Escape'])!;
    b.action(store);
    expect(store.state.mode).toBe('list');
  });

  it('clears searchQuery and returns from search to list', () => {
    const store = makeStore({ mode: 'search', searchQuery: 'hello' });
    const b = findBinding(['Escape'])!;
    b.action(store);
    expect(store.state.mode).toBe('list');
    expect(store.state.searchQuery).toBe('');
  });

  it('closes filterMenu when in list mode', () => {
    const store = makeStore({ mode: 'list', filterMenuOpen: true });
    const b = findBinding(['Escape'])!;
    b.action(store);
    expect(store.state.filterMenuOpen).toBe(false);
  });
});

describe('? binding — show help', () => {
  it('switches mode to help', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['?'])!;
    b.action(store);
    expect(store.state.mode).toBe('help');
  });
});

describe('/ binding — focus search', () => {
  it('switches mode to search', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['/'])!;
    b.action(store);
    expect(store.state.mode).toBe('search');
  });
});

describe('f binding — toggle filter menu', () => {
  it('opens filter menu and sets mode to filter when closed', () => {
    const store = makeStore({ filterMenuOpen: false, mode: 'list' });
    const b = findBinding(['f'])!;
    b.action(store);
    expect(store.state.filterMenuOpen).toBe(true);
    expect(store.state.mode).toBe('filter');
  });

  it('closes filter menu and returns to list mode when open', () => {
    const store = makeStore({ filterMenuOpen: true, mode: 'filter' });
    const b = findBinding(['f'])!;
    b.action(store);
    expect(store.state.filterMenuOpen).toBe(false);
    expect(store.state.mode).toBe('list');
  });
});

// ---------------------------------------------------------------------------
// Multi-key sequence tests
// ---------------------------------------------------------------------------

describe('gg binding — jump to top', () => {
  it('sets selectedPRIndex to 0', () => {
    const store = makeStore({ selectedPRIndex: 42 });
    const b = findBinding(['g', 'g'])!;
    b.action(store);
    expect(store.state.selectedPRIndex).toBe(0);
  });
});

describe('G binding — jump to bottom', () => {
  it('sets selectedPRIndex to MAX_SAFE_INTEGER', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    const b = findBinding(['G'])!;
    b.action(store);
    expect(store.state.selectedPRIndex).toBe(Number.MAX_SAFE_INTEGER);
  });
});

describe('aa binding — approve PR (double-tap)', () => {
  it('sets confirmingAction to "approve" when not currently confirming', () => {
    const store = makeStore({ mode: 'detail', confirmingAction: null });
    const b = findBinding(['a', 'a'])!;
    b.action(store);
    expect(store.state.confirmingAction).toBe('approve');
  });

  it('clears confirmingAction when already set to "approve" (second tap confirms)', () => {
    const store = makeStore({ mode: 'detail', confirmingAction: 'approve' });
    const b = findBinding(['a', 'a'])!;
    b.action(store);
    expect(store.state.confirmingAction).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Detail view navigation
// ---------------------------------------------------------------------------

describe('n binding — next file', () => {
  it('increments selectedFileIndex', () => {
    const store = makeStore({ mode: 'detail', selectedFileIndex: 1 });
    const b = findBinding(['n'])!;
    b.action(store);
    expect(store.state.selectedFileIndex).toBe(2);
  });
});

describe('p binding — previous file', () => {
  it('decrements selectedFileIndex', () => {
    const store = makeStore({ mode: 'detail', selectedFileIndex: 2 });
    const b = findBinding(['p'])!;
    b.action(store);
    expect(store.state.selectedFileIndex).toBe(1);
  });

  it('does not go below 0', () => {
    const store = makeStore({ mode: 'detail', selectedFileIndex: 0 });
    const b = findBinding(['p'])!;
    b.action(store);
    expect(store.state.selectedFileIndex).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// Mode-aware filtering
// ---------------------------------------------------------------------------

describe('mode-aware binding filtering', () => {
  it('list-only bindings have modes containing only list', () => {
    const jBinding = findBinding(['j'])!;
    expect(jBinding.modes).toEqual(['list']);
  });

  it('detail-only bindings have modes containing only detail', () => {
    const nBinding = findBinding(['n'])!;
    expect(nBinding.modes).toEqual(['detail']);
  });

  it('global bindings cover multiple modes', () => {
    const escBinding = findBinding(['Escape'])!;
    expect(escBinding.modes.length).toBeGreaterThan(1);
  });
});

// ---------------------------------------------------------------------------
// New list-only bindings added in compliance fix
// ---------------------------------------------------------------------------

describe('ctrl+d binding — page down', () => {
  it('increments selectedPRIndex by 5', () => {
    const store = makeStore({ selectedPRIndex: 10 });
    const b = findBinding(['ctrl+d'])!;
    b.action(store);
    expect(store.state.selectedPRIndex).toBe(15);
  });
});

describe('ctrl+u binding — page up', () => {
  it('decrements selectedPRIndex by 5', () => {
    const store = makeStore({ selectedPRIndex: 10 });
    const b = findBinding(['ctrl+u'])!;
    b.action(store);
    expect(store.state.selectedPRIndex).toBe(5);
  });

  it('does not go below 0', () => {
    const store = makeStore({ selectedPRIndex: 3 });
    const b = findBinding(['ctrl+u'])!;
    b.action(store);
    expect(store.state.selectedPRIndex).toBe(0);
  });
});

describe('o binding — open in browser', () => {
  it('sets confirmingAction to open-browser', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['o'])!;
    b.action(store);
    expect(store.state.confirmingAction).toBe('open-browser');
  });
});

describe('space binding — preview tooltip', () => {
  it('sets confirmingAction to preview', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding([' '])!;
    b.action(store);
    expect(store.state.confirmingAction).toBe('preview');
  });
});

describe('d binding — mark as draft', () => {
  it('sets confirmingAction to draft', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['d'])!;
    b.action(store);
    expect(store.state.confirmingAction).toBe('draft');
  });
});

describe('m binding — toggle mute', () => {
  it('sets confirmingAction to mute', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['m'])!;
    b.action(store);
    expect(store.state.confirmingAction).toBe('mute');
  });
});

describe('s binding — star PR', () => {
  it('sets confirmingAction to star', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['s'])!;
    b.action(store);
    expect(store.state.confirmingAction).toBe('star');
  });
});

describe(': binding — command mode', () => {
  it('sets mode to command', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding([':'])!;
    b.action(store);
    expect(store.state.mode).toBe('command');
  });
});

// ---------------------------------------------------------------------------
// Filter mode keyboard bindings
// ---------------------------------------------------------------------------

describe('filter mode keyboard flow', () => {
  it('o binding sets filterState to open in filter mode', () => {
    const result = findBindingFn('o', 'filter', undefined, undefined);
    expect(result).not.toBeNull();
    const store = makeStore({ mode: 'filter', filterState: 'all' });
    result!.binding.action(store);
    expect(store.state.filterState).toBe('open');
  });

  it('d binding sets filterState to draft in filter mode', () => {
    const result = findBindingFn('d', 'filter', undefined, undefined);
    expect(result).not.toBeNull();
    const store = makeStore({ mode: 'filter', filterState: 'open' });
    result!.binding.action(store);
    expect(store.state.filterState).toBe('draft');
  });

  it('a binding sets filterState to all in filter mode', () => {
    const result = findBindingFn('a', 'filter', undefined, undefined);
    expect(result).not.toBeNull();
    const store = makeStore({ mode: 'filter', filterState: 'open' });
    result!.binding.action(store);
    expect(store.state.filterState).toBe('all');
  });

  it('f binding closes filter menu in filter mode', () => {
    const result = findBindingFn('f', 'filter', undefined, undefined);
    expect(result).not.toBeNull();
    const store = makeStore({ mode: 'filter', filterMenuOpen: true });
    result!.binding.action(store);
    expect(store.state.filterMenuOpen).toBe(false);
    expect(store.state.mode).toBe('list');
  });

  it('Escape binding closes filter menu in filter mode', () => {
    const result = findBindingFn('Escape', 'filter', undefined, undefined);
    expect(result).not.toBeNull();
    const store = makeStore({ mode: 'filter', filterMenuOpen: true });
    result!.binding.action(store);
    expect(store.state.filterMenuOpen).toBe(false);
    expect(store.state.mode).toBe('list');
  });
});

// ---------------------------------------------------------------------------
// Platform search toggle bindings
// ---------------------------------------------------------------------------

describe('meta+/ binding — search toggle (macOS)', () => {
  it('enters search mode from list', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['meta+/'])!;
    b.action(store);
    expect(store.state.mode).toBe('search');
  });

  it('exits search mode back to list and clears query', () => {
    const store = makeStore({ mode: 'search', searchQuery: 'test' });
    const b = findBinding(['meta+/'])!;
    b.action(store);
    expect(store.state.mode).toBe('list');
    expect(store.state.searchQuery).toBe('');
  });
});

describe('ctrl+/ binding — search toggle (Linux/Windows)', () => {
  it('enters search mode from list', () => {
    const store = makeStore({ mode: 'list' });
    const b = findBinding(['ctrl+/'])!;
    b.action(store);
    expect(store.state.mode).toBe('search');
  });

  it('exits search mode back to list and clears query', () => {
    const store = makeStore({ mode: 'search', searchQuery: 'hello' });
    const b = findBinding(['ctrl+/'])!;
    b.action(store);
    expect(store.state.mode).toBe('list');
    expect(store.state.searchQuery).toBe('');
  });
});

// ---------------------------------------------------------------------------
// resolveEventKey — modifier combo resolution
// ---------------------------------------------------------------------------

describe('resolveEventKey', () => {
  function makeEvent(key: string, opts: Partial<KeyboardEventInit> = {}): KeyboardEvent {
    return new KeyboardEvent('keydown', { key, ...opts });
  }

  it('returns plain key for unmodified events', () => {
    expect(resolveEventKey(makeEvent('j'))).toBe('j');
    expect(resolveEventKey(makeEvent('Escape'))).toBe('Escape');
  });

  it('returns ctrl+key for ctrl-modified events', () => {
    expect(resolveEventKey(makeEvent('d', { ctrlKey: true }))).toBe('ctrl+d');
    expect(resolveEventKey(makeEvent('u', { ctrlKey: true }))).toBe('ctrl+u');
  });

  it('returns meta+key for meta-modified events', () => {
    expect(resolveEventKey(makeEvent('q', { metaKey: true }))).toBe('meta+q');
  });

  it('returns alt+key for alt-modified events', () => {
    expect(resolveEventKey(makeEvent('F4', { altKey: true }))).toBe('alt+F4');
  });

  it('returns the modifier key itself when only a modifier is pressed', () => {
    // e.g. pressing Ctrl alone: key='Control', ctrlKey=true
    expect(resolveEventKey(makeEvent('Control', { ctrlKey: true }))).toBe('Control');
  });
});

// ---------------------------------------------------------------------------
// findBinding (hook utility) — modifier combo matching
// ---------------------------------------------------------------------------

describe('findBinding (hook utility)', () => {
  it('matches ctrl+d modifier combo directly', () => {
    const result = findBindingFn('ctrl+d', 'list', undefined, undefined);
    expect(result).not.toBeNull();
    expect(result!.binding.keys).toEqual(['ctrl+d']);
  });

  it('matches ctrl+d in detail mode (detail scroll binding)', () => {
    const result = findBindingFn('ctrl+d', 'detail', undefined, undefined);
    expect(result).not.toBeNull();
    expect(result!.binding.modes).toContain('detail');
  });

  it('does not treat modifier combos as sequence starters', () => {
    // 'ctrl+d' contains '+', so it should never be stored as a sequence pending key
    const result = findBindingFn('ctrl+d', 'list', 'ctrl', Date.now());
    // Should match the modifier binding directly, not as a sequence
    expect(result).not.toBeNull();
    expect(result!.binding.keys).toEqual(['ctrl+d']);
  });

  it('matches gg as a timed sequence', () => {
    const result = findBindingFn('g', 'list', 'g', Date.now());
    expect(result).not.toBeNull();
    expect(result!.binding.keys).toEqual(['g', 'g']);
  });

  it('returns null for gg sequence when timeout has elapsed', () => {
    const staleTime = Date.now() - 600; // > 500ms timeout
    const result = findBindingFn('g', 'list', 'g', staleTime);
    // Falls through to single-key 'g' — which has no binding, so null
    expect(result).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// Cleanup — removeEventListener is called on unmount
// ---------------------------------------------------------------------------

describe('useKeyboard cleanup', () => {
  it('removes the keydown listener when the effect is cleaned up', async () => {
    const addSpy = vi.spyOn(window, 'addEventListener');
    const removeSpy = vi.spyOn(window, 'removeEventListener');

    // Dynamically import so the module uses the jsdom window
    const { useKeyboard } = await import('../../src/renderer/hooks/useKeyboard');

    // Simulate useEffect by calling the hook internals via React Testing Library
    // Since we are in node-less React context, we simulate the effect manually:
    // useKeyboard returns void; we instead test via React renderHook.
    const { renderHook } = await import('@testing-library/react');
    const { unmount } = renderHook(() => useKeyboard());

    // After mount, addEventListener should have been called with 'keydown'
    expect(addSpy).toHaveBeenCalledWith('keydown', expect.any(Function));

    // After unmount, removeEventListener should have been called with the same handler
    unmount();
    expect(removeSpy).toHaveBeenCalledWith('keydown', expect.any(Function));

    addSpy.mockRestore();
    removeSpy.mockRestore();
  });
});

// ---------------------------------------------------------------------------
// Input focus guard — keydown ignored when editable element has focus
// ---------------------------------------------------------------------------

describe('input focus guard', () => {
  it('ignores keydown when an <input> has focus', async () => {
    const { useKeyboard } = await import('../../src/renderer/hooks/useKeyboard');
    const { renderHook } = await import('@testing-library/react');

    renderHook(() => useKeyboard());

    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    // Dispatch 'j' — would normally increment selectedPRIndex
    const { appStore } = await import('../../src/renderer/store');
    const before = appStore.state.selectedPRIndex;
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', bubbles: true }));
    expect(appStore.state.selectedPRIndex).toBe(before);

    document.body.removeChild(input);
  });

  it('ignores keydown when a contenteditable element has focus', async () => {
    const { useKeyboard } = await import('../../src/renderer/hooks/useKeyboard');
    const { renderHook } = await import('@testing-library/react');

    renderHook(() => useKeyboard());

    const div = document.createElement('div');
    div.contentEditable = 'true';
    div.tabIndex = 0; // required for jsdom to accept focus on a div
    document.body.appendChild(div);
    div.focus();

    const { appStore } = await import('../../src/renderer/store');
    const before = appStore.state.selectedPRIndex;
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'j', bubbles: true }));
    expect(appStore.state.selectedPRIndex).toBe(before);

    document.body.removeChild(div);
  });

  it('still processes Escape even when an <input> has focus', async () => {
    const { useKeyboard } = await import('../../src/renderer/hooks/useKeyboard');
    const { renderHook } = await import('@testing-library/react');

    renderHook(() => useKeyboard());

    // Put store in help mode so Escape has an observable effect
    const { appStore } = await import('../../src/renderer/store');
    appStore.setState((s) => ({ ...s, mode: 'help' }));

    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    expect(appStore.state.mode).toBe('list');

    document.body.removeChild(input);
    // Reset store mode for subsequent tests
    appStore.setState((s) => ({ ...s, mode: 'list' }));
  });
});

// ---------------------------------------------------------------------------
// Detail view scroll bindings
// ---------------------------------------------------------------------------

describe('detail view scroll bindings', () => {
  it('h binding exists for detail mode (scroll left)', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === 'h' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
    expect(b!.description).toContain('left');
  });

  it('j binding exists for detail mode (scroll down)', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === 'j' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
  });

  it('k binding exists for detail mode (scroll up)', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === 'k' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
  });

  it('l binding exists for detail mode (scroll right)', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === 'l' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
    expect(b!.description).toContain('right');
  });

  it('0 binding jumps to top of diff', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === '0' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
    expect(b!.description).toContain('top');
  });

  it('$ binding jumps to bottom of diff', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === '$' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
    expect(b!.description).toContain('bottom');
  });

  it('ctrl+d binding exists for detail mode (page down)', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === 'ctrl+d' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
  });

  it('ctrl+u binding exists for detail mode (page up)', () => {
    const b = KEYBOARD_BINDING_GROUPS.find(
      (b) => b.keys[0] === 'ctrl+u' && b.modes.includes('detail')
    );
    expect(b).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// Immutability — store state is never mutated in place
// ---------------------------------------------------------------------------

describe('immutability', () => {
  it('j binding produces a new state object', () => {
    const store = makeStore({ selectedPRIndex: 0 });
    const before = store.state;
    const b = findBinding(['j'])!;
    b.action(store);
    expect(store.state).not.toBe(before);
    expect(before.selectedPRIndex).toBe(0); // original unchanged
  });

  it('k binding produces a new state object', () => {
    const store = makeStore({ selectedPRIndex: 5 });
    const before = store.state;
    const b = findBinding(['k'])!;
    b.action(store);
    expect(store.state).not.toBe(before);
  });
});
