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

  it('resolves VALIDATE_DEV_FOLDER requests when matching validation result arrives', async () => {
    const { result, unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const request = result.current.request('VALIDATE_DEV_FOLDER', {
      path: '/Users/sam/dev',
    });

    await vi.waitFor(() => {
      expect(sentMessagesOfType(ws, 'VALIDATE_DEV_FOLDER')).toHaveLength(1);
    });

    const validateRequest = sentMessagesOfType(ws, 'VALIDATE_DEV_FOLDER')[0];
    emitMessage(ws, {
      id: validateRequest?.id,
      type: 'VALIDATE_DEV_FOLDER_RESULT',
      data: {
        ok: true,
        canonical_path: '/private/var/dev',
      },
      timestamp: new Date().toISOString(),
    });

    await expect(request).resolves.toEqual({
      ok: true,
      canonical_path: '/private/var/dev',
    });

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
        ok: true,
        discovered: 0,
        healthy: 0,
        degraded: 0,
        skipped: 0,
        partial: false,
      },
      timestamp: new Date().toISOString(),
    });

    await expect(request).resolves.toEqual({
      ok: true,
      discovered: 0,
      healthy: 0,
      degraded: 0,
      skipped: 0,
      partial: false,
    });

    unmount();
  });

  it('resolves REQUEST_PR_SYNC only when a matching PR_SYNCED response arrives', async () => {
    const { result, unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const request = result.current.request('REQUEST_PR_SYNC', {
      pr_id: 'github:acme/widgets#42',
    });

    await vi.waitFor(() => {
      expect(sentMessagesOfType(ws, 'REQUEST_PR_SYNC')).toHaveLength(1);
    });

    const syncRequest = sentMessagesOfType(ws, 'REQUEST_PR_SYNC')[0];
    emitMessage(ws, {
      id: 'another-request',
      type: 'PR_SYNCED',
      data: { pr_id: 'github:acme/widgets#43' },
      timestamp: new Date().toISOString(),
    });
    emitMessage(ws, {
      id: syncRequest?.id,
      type: 'PR_SYNCED',
      data: { pr_id: 'github:acme/widgets#42' },
      timestamp: new Date().toISOString(),
    });

    await expect(request).resolves.toEqual({ pr_id: 'github:acme/widgets#42' });

    unmount();
  });

  it('resolves SET_EXPERTISE_SETTINGS requests when updated response arrives', async () => {
    const { result, unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();
    const payload = {
      alias_emails: [],
      min_surviving_lines: 50,
      excludes: { 'github:acme/api': [] },
    };

    const request = result.current.request('SET_EXPERTISE_SETTINGS', payload);

    await vi.waitFor(() => {
      expect(sentMessagesOfType(ws, 'SET_EXPERTISE_SETTINGS')).toHaveLength(1);
    });

    const settingsRequest = sentMessagesOfType(ws, 'SET_EXPERTISE_SETTINGS')[0];
    emitMessage(ws, {
      id: settingsRequest?.id,
      type: 'EXPERTISE_SETTINGS_UPDATED',
      data: payload,
      timestamp: new Date().toISOString(),
    });

    await expect(request).resolves.toEqual(payload);

    unmount();
  });

  it('resolves ADD_REPO_BY_URL requests when matching add-repo result arrives', async () => {
    const { result, unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const request = result.current.request('ADD_REPO_BY_URL', {
      url: 'https://github.com/acme/api.git',
      destination: null,
      dev_folder_path: '/Users/sam/dev',
    });

    await vi.waitFor(() => {
      expect(sentMessagesOfType(ws, 'ADD_REPO_BY_URL')).toHaveLength(1);
    });

    const addRepoRequest = sentMessagesOfType(ws, 'ADD_REPO_BY_URL')[0];
    emitMessage(ws, {
      id: addRepoRequest?.id,
      type: 'ADD_REPO_BY_URL_RESULT',
      data: {
        ok: true,
        source: 'local',
      },
      timestamp: new Date().toISOString(),
    });

    await expect(request).resolves.toEqual({
      ok: true,
      source: 'local',
    });

    unmount();
  });

  it.each([
    {
      requestType: 'RECOVER_REPO_PATH',
      resultType: 'RECOVER_REPO_PATH_RESULT',
      payload: { repo_id: 'github:acme/api', path: '/Users/sam/dev/api' },
      response: {
        ok: false,
        code: 'remote_mismatch',
        message: 'origin changed',
      },
    },
    {
      requestType: 'RECOVER_REPO_TO_CACHE',
      resultType: 'RECOVER_REPO_TO_CACHE_RESULT',
      payload: { repo_id: 'github:acme/api' },
      response: { ok: true },
    },
    {
      requestType: 'REMOVE_REPO_PREFLIGHT',
      resultType: 'REMOVE_REPO_PREFLIGHT_RESULT',
      payload: { repo_id: 'github:acme/api' },
      response: {
        ok: true,
        counts: { prs: 2, comments: 3, threads: 0, patchsets: 0, actions: 0 },
      },
    },
    {
      requestType: 'REMOVE_REPO',
      resultType: 'REMOVE_REPO_RESULT',
      payload: { repo_id: 'github:acme/api' },
      response: { ok: true },
    },
  ])('resolves $requestType requests when matching $resultType arrives', async ({
    requestType,
    resultType,
    payload,
    response,
  }) => {
    const { result, unmount } = renderHook(() => useWebSocket());
    const ws = openMountedWebSocket();

    const request = result.current.request(requestType, payload);

    await vi.waitFor(() => {
      expect(sentMessagesOfType(ws, requestType)).toHaveLength(1);
    });

    const sentRequest = sentMessagesOfType(ws, requestType)[0];
    emitMessage(ws, {
      id: sentRequest?.id,
      type: resultType,
      data: response,
      timestamp: new Date().toISOString(),
    });

    await expect(request).resolves.toEqual(response);

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
