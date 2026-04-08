import { describe, it, expect, beforeEach } from 'vitest';
import { appStore } from '../../src/renderer/store';

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
