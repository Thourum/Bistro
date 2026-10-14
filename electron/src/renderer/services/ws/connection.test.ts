import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { z } from 'zod';
import { wsClient } from './connection';

vi.mock('../../utils/logger', () => ({
  createLogger: () => ({
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  }),
}));

const CONNECT_WAIT_INTERVAL_MS = 250;
const REQUEST_TIMEOUT_MS = 5000;
const SOCKET_CLOSED = 3;

const sentMessageSchema = z.object({
  id: z.string(),
  type: z.string(),
  data: z.unknown(),
});
type SentMessage = z.infer<typeof sentMessageSchema>;

interface SocketControls {
  open: () => void;
  respond: (type: string, data: unknown) => void;
  sent: readonly SentMessage[];
}

function installSocket(): SocketControls {
  const state = { socket: null as FakeSocket | null };
  const sent: SentMessage[] = [];
  class FakeSocket {
    static readonly OPEN = 1;
    readyState = 0;
    onopen: (() => void) | null = null;
    onmessage: ((event: MessageEvent<string>) => void) | null = null;
    onerror: (() => void) | null = null;
    onclose: (() => void) | null = null;

    constructor() {
      state.socket = this;
    }

    send(message: string): void {
      const parsed = sentMessageSchema.safeParse(JSON.parse(message));
      expect(parsed.success).toBe(true);
      if (parsed.success) sent.push(parsed.data);
    }

    close(): void {
      this.readyState = SOCKET_CLOSED;
    }
  }
  vi.stubGlobal('WebSocket', FakeSocket);
  return {
    sent,
    open: () => {
      const socket = state.socket;
      if (!socket) return;
      socket.readyState = FakeSocket.OPEN;
      socket.onopen?.();
    },
    respond: (type, data) => {
      const request = sent.find((message) => message.type === 'REQUEST_DIFF');
      state.socket?.onmessage?.(
        new MessageEvent<string>('message', {
          data: JSON.stringify({ id: request?.id, type, data }),
        }),
      );
    },
  };
}

beforeEach(() => vi.useFakeTimers());

afterEach(() => {
  wsClient.disconnect();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('wsClient.request', () => {
  it('waits for an open socket and resolves a correlated response', async () => {
    const socket = installSocket();
    wsClient.connect();
    const response = wsClient.request('REQUEST_DIFF', { pr_id: 'pr-1' });
    expect(socket.sent).toHaveLength(0);

    socket.open();
    await vi.advanceTimersByTimeAsync(CONNECT_WAIT_INTERVAL_MS);
    socket.respond('DIFF_READY', { files: [] });

    expect((await response)._unsafeUnwrap()).toEqual({ files: [] });
    expect(vi.getTimerCount()).toBe(0);
  });

  it('returns the engine error as an EXTERNAL_ERROR without rejecting', async () => {
    const socket = installSocket();
    wsClient.connect();
    socket.open();
    const response = wsClient.request('REQUEST_DIFF', {});
    socket.respond('DIFF_READY', { error: 'diff not synced yet' });

    const fault = (await response)._unsafeUnwrapErr();
    expect(fault.tag).toBe('EXTERNAL_ERROR');
    expect(fault.message).toBe('diff not synced yet');
    expect(vi.getTimerCount()).toBe(0);
  });

  it('returns a TIMEOUT_ERROR when no correlated response arrives', async () => {
    const socket = installSocket();
    wsClient.connect();
    socket.open();
    const response = wsClient.request('REQUEST_DIFF', {});
    await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS);

    const fault = (await response)._unsafeUnwrapErr();
    expect(fault.tag).toBe('TIMEOUT_ERROR');
    expect(fault.message).toBe(
      'Timeout waiting for REQUEST_DIFF response (socket state: 1 — see bistro-sync.log for the backend side)',
    );
    expect(vi.getTimerCount()).toBe(0);
  });

  it('returns a CONNECTION_ERROR after the connection wait deadline', async () => {
    installSocket();
    const response = wsClient.request('REQUEST_DIFF', {});
    await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS);

    const fault = (await response)._unsafeUnwrapErr();
    expect(fault.tag).toBe('CONNECTION_ERROR');
    expect(fault.message).toBe(
      'WebSocket not connected (sending REQUEST_DIFF)',
    );
    expect(vi.getTimerCount()).toBe(0);
  });
});
