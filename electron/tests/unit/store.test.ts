import { describe, it, expect, beforeEach, vi } from 'vitest';
import type { UIState } from '../../src/renderer/store/ui';
import { createUIStore } from '../../src/renderer/store/ui';

describe('UIState action fields', () => {
  it('initializes with null pendingAction and actionResult', () => {
    const store = createUIStore();
    expect(store.state.pendingAction).toBeNull();
    expect(store.state.actionResult).toBeNull();
  });

  it('sets pendingAction immutably', () => {
    const store = createUIStore();
    const prev = store.state;
    store.setState((s) => ({
      ...s,
      pendingAction: { type: 'approve', prId: 'pr-1', startedAt: Date.now() },
    }));
    expect(store.state.pendingAction).toEqual(
      expect.objectContaining({ type: 'approve', prId: 'pr-1' })
    );
    expect(prev.pendingAction).toBeNull();
  });

  it('sets actionResult immutably', () => {
    const store = createUIStore();
    store.setState((s) => ({
      ...s,
      actionResult: { type: 'approve', prId: 'pr-1', success: true, message: 'Approved' },
    }));
    expect(store.state.actionResult).toEqual(
      expect.objectContaining({ success: true, message: 'Approved' })
    );
  });
});

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
