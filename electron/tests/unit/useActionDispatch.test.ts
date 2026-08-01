// @vitest-environment jsdom

import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../../src/renderer/store';

const mockSend = vi.fn();
vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    send: mockSend,
    request: vi.fn(),
    wsRef: { current: null },
  }),
}));

import { useActionDispatch } from '../../src/renderer/hooks/useActionDispatch';

describe('useActionDispatch', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    appStore.setState((s) => ({
      ...s,
      confirmingAction: null,
      pendingAction: null,
      actionResult: null,
    }));
  });

  it('dispatches APPROVE_PR and sets pendingAction', () => {
    const { result } = renderHook(() => useActionDispatch('pr-123'));

    act(() => {
      result.current.dispatchApprove();
    });

    expect(mockSend).toHaveBeenCalledWith('APPROVE_PR', { pr_id: 'pr-123' });
    expect(appStore.state.pendingAction).toEqual(
      expect.objectContaining({ type: 'approve', prId: 'pr-123' }),
    );
  });

  it('clears confirmingAction when dispatching approve', () => {
    appStore.setState((s) => ({ ...s, confirmingAction: 'approve_confirmed' }));
    const { result } = renderHook(() => useActionDispatch('pr-123'));

    act(() => {
      result.current.dispatchApprove();
    });

    expect(appStore.state.confirmingAction).toBeNull();
  });

  it('dispatches REQUEST_CHANGES with comment and sets pendingAction', () => {
    const { result } = renderHook(() => useActionDispatch('pr-456'));

    act(() => {
      result.current.dispatchRequestChanges('Needs fixes');
    });

    expect(mockSend).toHaveBeenCalledWith('REQUEST_CHANGES', {
      pr_id: 'pr-456',
      comment: 'Needs fixes',
    });
    expect(appStore.state.pendingAction).toEqual(
      expect.objectContaining({ type: 'request_changes', prId: 'pr-456' }),
    );
  });

  it('does nothing when prId is undefined', () => {
    const { result } = renderHook(() => useActionDispatch(undefined));

    act(() => {
      result.current.dispatchApprove();
    });

    expect(mockSend).not.toHaveBeenCalled();
    expect(appStore.state.pendingAction).toBeNull();
  });

  it('clearActionResult resets actionResult to null', () => {
    appStore.setState((s) => ({
      ...s,
      actionResult: {
        type: 'approve',
        prId: 'pr-1',
        success: true,
        message: 'Done',
      },
    }));
    const { result } = renderHook(() => useActionDispatch('pr-1'));

    act(() => {
      result.current.clearActionResult();
    });

    expect(appStore.state.actionResult).toBeNull();
  });

  it('pendingAction is set immutably', () => {
    const { result } = renderHook(() => useActionDispatch('pr-123'));
    const prev = appStore.state;

    act(() => {
      result.current.dispatchApprove();
    });

    expect(appStore.state).not.toBe(prev);
    expect(prev.pendingAction).toBeNull();
  });
});
