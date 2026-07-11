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

function sentMessagesOfType<TData>(
  ws: MockWebSocket,
  type: string,
): Array<{ type: string; data: TData }> {
  return ws.sentMessages
    .map((message) => JSON.parse(message) as { type: string; data: TData })
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

    unmount();
  });

  it('does not send SCAN_DEV_FOLDER on connection open when devFolderPath is null', () => {
    const { unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const scanMessages = sentMessagesOfType(ws, 'SCAN_DEV_FOLDER');

    expect(scanMessages).toHaveLength(0);

    unmount();
  });
});
