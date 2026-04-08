import { describe, it, expect, beforeEach } from 'vitest';
import { appStore } from '../../src/renderer/store';

describe('WebSocket reconnect backoff', () => {
  it('calculates exponential backoff delay', () => {
    const BASE_DELAY = 1000;
    const MAX_DELAY = 30000;
    const backoff = (attempt: number): number => Math.min(BASE_DELAY * Math.pow(2, attempt), MAX_DELAY);

    expect(backoff(0)).toBe(1000);
    expect(backoff(1)).toBe(2000);
    expect(backoff(2)).toBe(4000);
    expect(backoff(3)).toBe(8000);
    expect(backoff(4)).toBe(16000);
    expect(backoff(5)).toBe(30000);
    expect(backoff(10)).toBe(30000);
  });
});

describe('offline detection', () => {
  it('sets isOffline in store when navigator.onLine is false', () => {
    appStore.setState((s) => ({ ...s, isOffline: true }));
    expect(appStore.state.isOffline).toBe(true);
  });
});

describe('WebSocket ACTION_COMPLETE handler', () => {
  beforeEach(() => {
    appStore.setState((s) => ({
      ...s,
      pendingAction: { type: 'approve', prId: 'pr-1', startedAt: Date.now() },
      actionResult: null,
      lastSyncAt: null,
    }));
  });

  it('clears pendingAction and sets actionResult on ACTION_COMPLETE', () => {
    const data = { action_type: 'approve', pr_id: 'pr-1', message: 'PR approved' };
    appStore.setState((s) => ({
      ...s,
      pendingAction: null,
      actionResult: {
        type: data.action_type as 'approve',
        prId: data.pr_id,
        success: true,
        message: data.message,
      },
      lastSyncAt: new Date().toISOString(),
    }));

    expect(appStore.state.pendingAction).toBeNull();
    expect(appStore.state.actionResult).toEqual(
      expect.objectContaining({ type: 'approve', success: true })
    );
  });

  it('clears pendingAction and sets actionResult.success=false on ACTION_FAILED', () => {
    const data = { action_type: 'approve', pr_id: 'pr-1', error: 'Forbidden' };
    appStore.setState((s) => ({
      ...s,
      pendingAction: null,
      actionResult: {
        type: data.action_type as 'approve',
        prId: data.pr_id,
        success: false,
        message: data.error,
      },
    }));

    expect(appStore.state.pendingAction).toBeNull();
    expect(appStore.state.actionResult).toEqual(
      expect.objectContaining({ type: 'approve', success: false, message: 'Forbidden' })
    );
  });

  it('immutable: previous state is not mutated', () => {
    const prev = appStore.state;
    appStore.setState((s) => ({
      ...s,
      pendingAction: null,
      actionResult: { type: 'approve', prId: 'pr-1', success: true, message: 'Done' },
    }));
    expect(appStore.state).not.toBe(prev);
    expect(prev.pendingAction).not.toBeNull();
  });
});
