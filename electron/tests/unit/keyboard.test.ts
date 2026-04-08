import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { Store } from '@tanstack/store';
import type { UIState } from '../../src/renderer/store/ui';
import { KEYBOARD_BINDING_GROUPS, bindingSequence } from '../../src/renderer/utils/keyboard';
import type { BindingDefinition } from '../../src/renderer/utils/keyboard';

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
  it('exports a non-empty array of binding definitions', () => {
    expect(KEYBOARD_BINDING_GROUPS.length).toBeGreaterThan(0);
  });

  it('every binding has keys, description, modes, and action', () => {
    for (const b of KEYBOARD_BINDING_GROUPS) {
      expect(Array.isArray(b.keys)).toBe(true);
      expect(b.keys.length).toBeGreaterThanOrEqual(1);
      expect(typeof b.description).toBe('string');
      expect(Array.isArray(b.modes)).toBe(true);
      expect(typeof b.action).toBe('function');
    }
  });

  it('includes all expected single-key list bindings', () => {
    const expectedKeys = ['j', 'k', 'G', 'Enter', '/', 'f'];
    for (const key of expectedKeys) {
      const b = findBinding([key]);
      expect(b, `expected binding for key "${key}"`).toBeDefined();
    }
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
  it('opens filter menu when closed', () => {
    const store = makeStore({ filterMenuOpen: false });
    const b = findBinding(['f'])!;
    b.action(store);
    expect(store.state.filterMenuOpen).toBe(true);
  });

  it('closes filter menu when open', () => {
    const store = makeStore({ filterMenuOpen: true });
    const b = findBinding(['f'])!;
    b.action(store);
    expect(store.state.filterMenuOpen).toBe(false);
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
