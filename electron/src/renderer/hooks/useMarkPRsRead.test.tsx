// @vitest-environment jsdom

import { renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useMarkPRsRead } from './useMarkPRsRead';

const mockRequest = vi.hoisted(() => vi.fn());

vi.mock('./useWebSocket', () => ({
  useWebSocket: () => ({ request: mockRequest }),
}));

describe('useMarkPRsRead', () => {
  beforeEach(() => {
    mockRequest.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('returns the engine acknowledgement', async () => {
    const acknowledgement = { ok: true, marked: 2 };
    mockRequest.mockResolvedValue(acknowledgement);
    const { result } = renderHook(() => useMarkPRsRead());

    await expect(result.current.markPRsRead(['pr-1', 'pr-2'])).resolves.toEqual(
      acknowledgement,
    );
    expect(mockRequest).toHaveBeenCalledWith('MARK_PRS_READ', {
      pr_ids: ['pr-1', 'pr-2'],
    });
  });

  it('returns null when the request fails', async () => {
    mockRequest.mockRejectedValue(new Error('WebSocket unavailable'));
    const { result } = renderHook(() => useMarkPRsRead());

    await expect(result.current.markPRsRead(['pr-1'])).resolves.toBeNull();
  });
});
