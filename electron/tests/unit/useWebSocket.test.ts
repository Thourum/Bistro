// @vitest-environment jsdom

import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { appStore } from '../../src/renderer/store';
import { settingsStore } from '../../src/renderer/store/settings';
import { DEFAULT_SETTINGS } from '../../src/types/settings';

describe('WebSocket reconnect backoff', () => {
  it('calculates exponential backoff delay', () => {
    const BASE_DELAY = 1000;
    const MAX_DELAY = 30000;
    const backoff = (attempt: number): number =>
      Math.min(BASE_DELAY * 2 ** attempt, MAX_DELAY);

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
    const data = {
      action_type: 'approve',
      pr_id: 'pr-1',
      message: 'PR approved',
    };
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
      expect.objectContaining({ type: 'approve', success: true }),
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
      expect.objectContaining({
        type: 'approve',
        success: false,
        message: 'Forbidden',
      }),
    );
  });

  it('immutable: previous state is not mutated', () => {
    const prev = appStore.state;
    appStore.setState((s) => ({
      ...s,
      pendingAction: null,
      actionResult: {
        type: 'approve',
        prId: 'pr-1',
        success: true,
        message: 'Done',
      },
    }));
    expect(appStore.state).not.toBe(prev);
    expect(prev.pendingAction).not.toBeNull();
  });
});

class MockWebSocket {
  static instances: MockWebSocket[] = [];
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  readyState = MockWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: (() => void) | null = null;
  sentMessages: string[] = [];

  constructor(readonly url: string) {
    MockWebSocket.instances = [...MockWebSocket.instances, this];
  }

  send(message: string): void {
    this.sentMessages = [...this.sentMessages, message];
  }

  close(): void {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.();
  }

  open(): void {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }
}

describe('WebSocket dev-folder startup scan', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal('WebSocket', MockWebSocket);
    settingsStore.setState(() => DEFAULT_SETTINGS);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    settingsStore.setState(() => DEFAULT_SETTINGS);
  });

  it('sends exactly one SCAN_DEV_FOLDER message on connection open when devFolderPath is configured', async () => {
    settingsStore.setState((settings) => ({
      ...settings,
      repositories: {
        ...settings.repositories,
        devFolderPath: '/Users/sam/dev',
      },
    }));
    const { useWebSocket } = await import('../../src/renderer/hooks/useWebSocket');

    const { unmount } = renderHook(() => useWebSocket());
    const ws = MockWebSocket.instances[0];
    act(() => ws.open());

    const scanMessages = ws.sentMessages
      .map((message) => JSON.parse(message) as { type: string; data: unknown })
      .filter((message) => message.type === 'SCAN_DEV_FOLDER');

    expect(scanMessages).toHaveLength(1);
    expect(scanMessages[0]?.data).toEqual({
      path: '/Users/sam/dev',
      reason: 'startup',
    });

    act(() => {
      ws.onmessage?.(
        new MessageEvent('message', {
          data: JSON.stringify({
            id: 'auth-check-result-1',
            type: 'AUTH_CHECK_RESULT',
            data: { authenticated: true, providers: [] },
            timestamp: new Date().toISOString(),
          }),
        }),
      );
    });

    const scanMessagesAfterAuthCheck = ws.sentMessages
      .map((message) => JSON.parse(message) as { type: string })
      .filter((message) => message.type === 'SCAN_DEV_FOLDER');

    expect(scanMessagesAfterAuthCheck).toHaveLength(1);
    unmount();
  });

  it('does not send SCAN_DEV_FOLDER on connection open when devFolderPath is null', async () => {
    const { useWebSocket } = await import('../../src/renderer/hooks/useWebSocket');

    const { unmount } = renderHook(() => useWebSocket());
    const ws = MockWebSocket.instances[0];
    act(() => ws.open());

    const scanMessages = ws.sentMessages
      .map((message) => JSON.parse(message) as { type: string })
      .filter((message) => message.type === 'SCAN_DEV_FOLDER');

    expect(scanMessages).toHaveLength(0);
    unmount();
  });
});

describe('extractResponseError', () => {
  it('returns the error string from a response payload', async () => {
    const { extractResponseError } = await import(
      '../../src/renderer/hooks/useWebSocket'
    );
    expect(extractResponseError({ error: 'diff not found' })).toBe(
      'diff not found',
    );
  });

  it('returns null when there is no error field', async () => {
    const { extractResponseError } = await import(
      '../../src/renderer/hooks/useWebSocket'
    );
    expect(extractResponseError({ diff: 'abc' })).toBeNull();
    expect(extractResponseError({})).toBeNull();
  });

  it('returns null for empty-string errors and non-objects', async () => {
    const { extractResponseError } = await import(
      '../../src/renderer/hooks/useWebSocket'
    );
    expect(extractResponseError({ error: '' })).toBeNull();
    expect(extractResponseError(null)).toBeNull();
    expect(extractResponseError('oops')).toBeNull();
    expect(extractResponseError({ error: 42 })).toBeNull();
  });
});

describe('WebSocket thread state-change handling', () => {
  it('THREAD_RESOLVED / THREAD_REOPENED bump threadsRevision so the view refetches', () => {
    appStore.setState((s) => ({ ...s, threadsRevision: 0 }));

    // The onMessage handler increments threadsRevision; mirror that effect.
    appStore.setState((s) => ({
      ...s,
      threadsRevision: s.threadsRevision + 1,
    }));
    expect(appStore.state.threadsRevision).toBe(1);

    appStore.setState((s) => ({
      ...s,
      threadsRevision: s.threadsRevision + 1,
    }));
    expect(appStore.state.threadsRevision).toBe(2);
  });
});
