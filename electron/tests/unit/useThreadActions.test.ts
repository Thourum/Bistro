// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { mockRequest, mockAddToast } = vi.hoisted(() => ({
  mockRequest: vi.fn(),
  mockAddToast: vi.fn(),
}));

vi.mock('../../src/renderer/hooks/useWebSocket', () => ({
  useWebSocket: () => ({
    request: mockRequest,
  }),
}));

vi.mock('../../src/renderer/components/ToastContainer', () => ({
  addToast: mockAddToast,
}));

import { useThreadActions } from '../../src/renderer/hooks/useThreadActions';

describe('useThreadActions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('submits resolve_thread through SUBMIT_ACTION and clears pending on success', async () => {
    let resolveRequest: (value: unknown) => void = () => undefined;
    mockRequest.mockReturnValue(
      new Promise((resolve) => {
        resolveRequest = resolve;
      }),
    );
    const { result } = renderHook(() => useThreadActions('pr-1'));

    act(() => {
      result.current.resolveThread('PRRT_1');
    });

    expect(result.current.isThreadPending('PRRT_1')).toBe(true);

    await act(async () => {
      resolveRequest({ success: true });
      await Promise.resolve();
    });

    expect(mockRequest).toHaveBeenCalledWith('SUBMIT_ACTION', {
      pr_id: 'pr-1',
      action: 'resolve_thread',
      thread_id: 'PRRT_1',
      body: 'PRRT_1',
    });
    expect(result.current.isThreadPending('PRRT_1')).toBe(false);
    expect(mockAddToast).toHaveBeenCalledWith({
      type: 'success',
      message: 'Thread resolved',
      dismissible: false,
    });
  });

  it('shows an error toast and clears pending when unresolve fails', async () => {
    mockRequest.mockRejectedValue(new Error('thread lookup failed'));
    const { result } = renderHook(() => useThreadActions('pr-1'));

    await act(async () => {
      result.current.unresolveThread('PRRT_2');
      await Promise.resolve();
    });

    expect(result.current.isThreadPending('PRRT_2')).toBe(false);
    expect(mockAddToast).toHaveBeenCalledWith({
      type: 'error',
      message: 'thread lookup failed',
      dismissible: true,
    });
  });

  it('shows queued feedback for rate-limited offline replay', async () => {
    mockRequest.mockRejectedValue(
      new Error('rate limited: action queued for later replay'),
    );
    const { result } = renderHook(() => useThreadActions('pr-1'));

    await act(async () => {
      result.current.resolveThread('PRRT_3');
      await Promise.resolve();
    });

    expect(mockAddToast).toHaveBeenCalledWith({
      type: 'warning',
      message: 'Thread resolve queued for retry',
      dismissible: true,
    });
  });
});
