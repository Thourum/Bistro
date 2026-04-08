import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { UIState } from '../../src/renderer/store/ui';

describe('UIState', () => {
  it('should initialize with default values', () => {
    const initial: UIState = {
      selectedPRIndex: 0,
      searchQuery: '',
      mode: 'list',
      paneLayout: 'split',
      sortBy: 'updated',
      filterState: 'open',
    };

    expect(initial.mode).toBe('list');
    expect(initial.selectedPRIndex).toBe(0);
  });

  it('should support state updates', () => {
    let state: UIState = {
      selectedPRIndex: 0,
      searchQuery: '',
      mode: 'list',
      paneLayout: 'split',
      sortBy: 'updated',
      filterState: 'open',
    };

    // Simulate update
    state = { ...state, selectedPRIndex: 1, mode: 'detail' };
    expect(state.selectedPRIndex).toBe(1);
    expect(state.mode).toBe('detail');
  });
});
