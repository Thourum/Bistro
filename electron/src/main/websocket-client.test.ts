import type { Logger } from 'electron-log';
import { expect, it, vi } from 'vitest';
import { WebSocketClient } from './websocket-client';

it('refuses main-process writes before connection in keyboard benchmark mode', () => {
  const logger = {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  } as unknown as Logger;
  const socket = new WebSocketClient('ws://127.0.0.1:8080/ws', logger, {
    isKeysBench: true,
  });
  for (const message of [
    { type: 'SET_SYNC_SETTINGS', data: { auto_sync: true } },
    {
      type: 'SET_CLASSIFICATION_SETTINGS',
      data: { mode: 'rules+llm', dismissal_scope: 'local' },
    },
    { type: 'SUBMIT_ACTION', data: {} },
    { type: 'REQUEST_UNKNOWN', data: {} },
  ]) {
    expect(socket.send(message)._unsafeUnwrapErr().tag).toBe(
      'BENCH_WRITE_BLOCKED',
    );
  }
  expect(
    socket.send({ type: 'AUTH_CHECK', data: {} })._unsafeUnwrapErr().tag,
  ).toBe('CONNECTION_ERROR');
  expect(
    socket
      .send({
        type: 'SET_CLASSIFICATION_SETTINGS',
        data: { mode: 'rules', dismissal_scope: 'local' },
      })
      ._unsafeUnwrapErr().tag,
  ).toBe('CONNECTION_ERROR');
  const ordinary = new WebSocketClient('ws://127.0.0.1:8080/ws', logger);
  expect(
    ordinary.send({ type: 'SUBMIT_ACTION', data: {} })._unsafeUnwrapErr().tag,
  ).toBe('CONNECTION_ERROR');
});
