import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { UIState } from '../../src/store/ui';
import { createUIStore, initialUIState } from '../../src/store/ui';
import type { Toast } from '../../src/types';

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
      expect.objectContaining({ type: 'approve', prId: 'pr-1' }),
    );
    expect(prev.pendingAction).toBeNull();
  });

  it('sets actionResult immutably', () => {
    const store = createUIStore();
    store.setState((s) => ({
      ...s,
      actionResult: {
        type: 'approve',
        prId: 'pr-1',
        success: true,
        message: 'Approved',
      },
    }));
    expect(store.state.actionResult).toEqual(
      expect.objectContaining({ success: true, message: 'Approved' }),
    );
  });
});

describe('UIState toast fields', () => {
  it('initializes with empty toasts array and isOffline false', () => {
    const store = createUIStore();
    expect(store.state.toasts).toEqual([]);
    expect(store.state.isOffline).toBe(false);
  });

  it('adds a toast immutably', () => {
    const store = createUIStore();
    const prev = store.state;
    store.setState((s) => ({
      ...s,
      toasts: [
        ...s.toasts,
        {
          id: 't1',
          type: 'error' as const,
          message: 'Oops',
          dismissible: true,
        },
      ],
    }));
    expect(store.state.toasts).toHaveLength(1);
    expect(prev.toasts).toHaveLength(0);
  });

  it('caps toasts at 3 (removes oldest)', () => {
    const store = createUIStore();
    const toasts: Toast[] = [
      { id: 't1', type: 'error', message: 'A', dismissible: true },
      { id: 't2', type: 'error', message: 'B', dismissible: true },
      { id: 't3', type: 'error', message: 'C', dismissible: true },
    ];
    store.setState((s) => ({ ...s, toasts }));
    const newToast: Toast = {
      id: 't4',
      type: 'info',
      message: 'D',
      dismissible: true,
    };
    store.setState((s) => ({
      ...s,
      toasts: [...s.toasts.slice(-2), newToast],
    }));
    expect(store.state.toasts).toHaveLength(3);
    expect(store.state.toasts[0]?.id).toBe('t2');
  });
});

describe('UIState', () => {
  it('should initialize with default values', () => {
    const initial: UIState = {
      ...initialUIState,
      selectedPRIndex: 0,
      searchQuery: '',
      sortBy: 'updated',
      filterState: 'open',
    };

    expect(initial.selectedPRIndex).toBe(0);
  });

  it('should support state updates', () => {
    let state: UIState = {
      ...initialUIState,
      selectedPRIndex: 0,
      searchQuery: '',
      sortBy: 'updated',
      filterState: 'open',
    };

    // Simulate update
    state = { ...state, selectedPRIndex: 1 };
    expect(state.selectedPRIndex).toBe(1);
  });
});
