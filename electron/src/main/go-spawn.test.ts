import { ServiceError } from '@thourum/fault';
import type { Logger } from 'electron-log';
import { afterEach, describe, expect, it, vi } from 'vitest';
import waitOn from 'wait-on';
import { SLOW_ENGINE_MS } from '../types/engine';
import {
  createGoReadyWaiter,
  parseReadyLine,
  spawnGoProcess,
} from './go-spawn';

vi.mock('wait-on', () => ({ default: vi.fn() }));

describe('parseReadyLine', () => {
  it('accepts the Go ready signal and rejects an invalid port', () => {
    expect(parseReadyLine('{"event":"ready","wsPort":64568}')).toBe(64568);
    expect(parseReadyLine('{"event":"ready","wsPort":65536}')).toBeNull();
    expect(parseReadyLine('{"event":"ready","wsPort":"64568"}')).toBeNull();
  });
});

it('keeps observing a slow external engine until its port is ready', async () => {
  vi.useFakeTimers();
  vi.stubEnv('BISTRO_SKIP_GO_SPAWN', '1');
  const externalPort = 8080;
  vi.mocked(waitOn).mockImplementationOnce(
    (options) =>
      new Promise<void>((resolve, reject) => {
        const readyTimer = setTimeout(resolve, SLOW_ENGINE_MS * 3);
        if (Number.isFinite(options.timeout))
          setTimeout(() => {
            clearTimeout(readyTimer);
            reject(new Error('external engine timed out'));
          }, options.timeout);
      }),
  );
  const onStillStarting = vi.fn();
  const onReady = vi.fn();
  const startup = spawnGoProcess({
    config: {
      binaryPath: '',
      dbPath: '',
      wsPort: externalPort,
      logLevel: 'info',
      logFile: '',
      replayQueue: false,
      syncInterval: 30,
      autoSync: false,
    },
    logger: {
      info: vi.fn(),
      warn: vi.fn(),
      error: vi.fn(),
    } as unknown as Logger,
    onStillStarting,
    onReady,
  });
  const settled = startup.then(
    (port) => port,
    () => null,
  );
  await vi.advanceTimersByTimeAsync(SLOW_ENGINE_MS);
  expect(onStillStarting).toHaveBeenCalledOnce();
  expect(onReady).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(SLOW_ENGINE_MS * 2);
  expect(await settled).toBe(externalPort);
  expect(onReady).toHaveBeenCalledWith(externalPort);
  vi.unstubAllEnvs();
  vi.useRealTimers();
});

describe('createGoReadyWaiter', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('keeps waiting for a ready signal after reporting a slow startup', async () => {
    vi.useFakeTimers();
    const onStillStarting = vi.fn();
    const waiter = createGoReadyWaiter(15_000, onStillStarting);

    const readyPort = waiter.wait();
    await vi.advanceTimersByTimeAsync(15_000);

    expect(onStillStarting).toHaveBeenCalledOnce();
    waiter.markReady(64568);

    expect((await readyPort)._unsafeUnwrap()).toBe(64568);
  });

  it('returns err when the process exits before it becomes ready', async () => {
    const waiter = createGoReadyWaiter(15_000, vi.fn());
    const readyPort = waiter.wait();

    waiter.markFailed(
      ServiceError('GO_PROCESS_ERROR', 'Go process exited prematurely'),
    );

    const error = (await readyPort)._unsafeUnwrapErr();
    expect(error.message).toBe('Go process exited prematurely');
    expect(error.tag).toBe('GO_PROCESS_ERROR');
  });
});
