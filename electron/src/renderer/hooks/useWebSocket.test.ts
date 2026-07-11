// @vitest-environment jsdom

import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DEFAULT_SETTINGS } from '../../types/settings';
import { settingsStore } from '../store/settings';
import { useWebSocket } from './useWebSocket';

class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readonly url: string;
  readyState = MockWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onclose: (() => void) | null = null;
  sentMessages: string[] = [];

  constructor(url: string) {
    this.url = url;
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

function getMountedWebSocket(): MockWebSocket {
  const ws = MockWebSocket.instances[0];
  if (ws === undefined) {
    throw new Error('Expected mounted WebSocket instance');
  }
  return ws;
}

function openMountedWebSocket(): MockWebSocket {
  const ws = getMountedWebSocket();
  act(() => ws.open());
  return ws;
}

function emitMessage(ws: MockWebSocket, message: unknown): void {
  act(() => {
    ws.onmessage?.(
      new MessageEvent('message', {
        data: JSON.stringify(message),
      }),
    );
  });
}

function emitAuthCheckResult(ws: MockWebSocket): void {
  emitMessage(ws, {
    id: 'auth-check-result-1',
    type: 'AUTH_CHECK_RESULT',
    data: { authenticated: true, providers: [] },
    timestamp: new Date().toISOString(),
  });
}

function sentMessagesOfType<TData>(
  ws: MockWebSocket,
  type: string,
): Array<{ id: string; type: string; data: TData }> {
  return ws.sentMessages
    .map(
      (message) =>
        JSON.parse(message) as { id: string; type: string; data: TData },
    )
    .filter((message) => message.type === type);
}

describe('useWebSocket dev-folder startup scan', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal('WebSocket', MockWebSocket);
    settingsStore.setState(() => DEFAULT_SETTINGS);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    settingsStore.setState(() => DEFAULT_SETTINGS);
  });

  it('sends exactly one SCAN_DEV_FOLDER message on connection open when devFolderPath is set', () => {
    settingsStore.setState((settings) => ({
      ...settings,
      repositories: {
        ...settings.repositories,
        devFolderPath: '/Users/sam/dev',
      },
    }));

    const { unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const scanMessages = sentMessagesOfType<{
      path: string;
      reason: 'startup';
    }>(ws, 'SCAN_DEV_FOLDER');

    expect(scanMessages).toHaveLength(1);
    expect(scanMessages[0]?.data).toEqual({
      path: '/Users/sam/dev',
      reason: 'startup',
    });

    emitAuthCheckResult(ws);

    expect(sentMessagesOfType(ws, 'SCAN_DEV_FOLDER')).toHaveLength(1);

    unmount();
  });

  it('does not send SCAN_DEV_FOLDER on connection open when devFolderPath is null', () => {
    const { unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const scanMessages = sentMessagesOfType(ws, 'SCAN_DEV_FOLDER');

    expect(scanMessages).toHaveLength(0);

    unmount();
  });

  it('does not send SCAN_DEV_FOLDER on connection open when devFolderPath is empty', () => {
    settingsStore.setState((settings) => ({
      ...settings,
      repositories: {
        ...settings.repositories,
        devFolderPath: '',
      },
    }));

    const { unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const scanMessages = sentMessagesOfType(ws, 'SCAN_DEV_FOLDER');

    expect(scanMessages).toHaveLength(0);

    unmount();
  });

  it('resolves SCAN_DEV_FOLDER requests when matching scan result arrives', async () => {
    const { result, unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const request = result.current.request('SCAN_DEV_FOLDER', {
      path: '/Users/sam/dev',
      reason: 'manual',
    });

    await vi.waitFor(() => {
      expect(sentMessagesOfType(ws, 'SCAN_DEV_FOLDER')).toHaveLength(1);
    });

    const scanRequest = sentMessagesOfType(ws, 'SCAN_DEV_FOLDER')[0];
    emitMessage(ws, {
      id: scanRequest?.id,
      type: 'SCAN_DEV_FOLDER_RESULT',
      data: {
        path: '/Users/sam/dev',
        repositories: [],
        errors: [],
      },
      timestamp: new Date().toISOString(),
    });

    await expect(request).resolves.toEqual({
      path: '/Users/sam/dev',
      repositories: [],
      errors: [],
    });

    unmount();
  });

  it('ignores malformed SCAN_DEV_FOLDER_RESULT payloads without throwing', () => {
    const { unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    expect(() => {
      emitMessage(ws, {
        id: 'scan-result-1',
        type: 'SCAN_DEV_FOLDER_RESULT',
        data: { path: '/Users/sam/dev' },
        timestamp: new Date().toISOString(),
      });
    }).not.toThrow();

    unmount();
  });

  it('ignores malformed LOCATION_ERROR payloads without throwing', () => {
    const { unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    expect(() => {
      emitMessage(ws, {
        id: 'location-error-1',
        type: 'LOCATION_ERROR',
        data: { repo_id: 'github:acme/api', operation: 'fetch' },
        timestamp: new Date().toISOString(),
      });
    }).not.toThrow();

    unmount();
  });
});
