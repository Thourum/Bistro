import { EventEmitter, once as onceEvent } from 'node:events';
import { createServer, type Server } from 'node:net';
import { PassThrough } from 'node:stream';
import type { Logger } from 'electron-log';
import type { ResultPromise } from 'execa';
import { execa } from 'execa';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  isGoProcessRunning,
  killGoProcess,
  spawnGoProcess,
} from '../../src/main/go-spawn';
import type { GoProcessConfig } from '../../src/main/types';

vi.mock('execa', () => ({
  execa: vi.fn(),
}));

const execaMock = vi.mocked(execa);

type MockLogger = Pick<Logger, 'info' | 'warn' | 'error'>;

type MockProcess = ResultPromise & {
  stdout: PassThrough;
  stderr: PassThrough;
  killed: boolean;
  kill: ReturnType<typeof vi.fn>;
  resolveExit: (exitCode?: number, signal?: string) => void;
  rejectExit: (error: Error & { exitCode?: number; signal?: string }) => void;
};

const createExitResult = (
  exitCode?: number,
  signal?: string,
): { exitCode?: number; signal?: string } => ({
  ...(exitCode === undefined ? {} : { exitCode }),
  ...(signal === undefined ? {} : { signal }),
});

const createMockProcess = (): MockProcess => {
  const emitter = new EventEmitter();
  const stdout = new PassThrough();
  const stderr = new PassThrough();
  let resolveExit: (value: { exitCode?: number; signal?: string }) => void =
    () => {};
  let rejectExit: (
    error: Error & { exitCode?: number; signal?: string },
  ) => void = () => {};
  const exitPromise = new Promise<{ exitCode?: number; signal?: string }>(
    (resolve, reject) => {
      resolveExit = resolve;
      rejectExit = reject;
    },
  );

  const processPromise = Object.assign(exitPromise, {
    stdout,
    stderr,
    killed: false,
    on: emitter.on.bind(emitter),
    once: emitter.once.bind(emitter),
    emit: emitter.emit.bind(emitter),
    kill: vi.fn(function kill(this: MockProcess, signal?: string) {
      this.killed = true;
      resolveExit(createExitResult(undefined, signal));
      return true;
    }),
    resolveExit: (exitCode?: number, signal?: string) => {
      resolveExit(createExitResult(exitCode, signal));
    },
    rejectExit,
  });

  return processPromise as unknown as MockProcess;
};

describe('Go Process Spawn', () => {
  let logger: MockLogger;
  let config: GoProcessConfig;
  let server: Server | null;

  beforeEach(() => {
    logger = { info: vi.fn(), warn: vi.fn(), error: vi.fn() };
    config = {
      binaryPath: '/fake/path/bistro-sync',
      dbPath: '/fake/path/bistro.db',
      wsPort: 8080,
      logLevel: 'info',
      replayQueue: true,
      syncInterval: 60,
      autoSync: true,
      logFile: '',
    };
    server = null;
    execaMock.mockReset();
    delete process.env.BISTRO_SKIP_GO_SPAWN;
    killGoProcess();
  });

  afterEach(() => {
    delete process.env.BISTRO_SKIP_GO_SPAWN;
    if (isGoProcessRunning()) {
      killGoProcess();
    }
    server?.close();
  });

  it('spawns Go with execa and resolves from the ready JSON stdout line', async () => {
    const mockProcess = createMockProcess();
    execaMock.mockReturnValue(mockProcess);

    const portPromise = spawnGoProcess({ config, logger: logger as Logger });
    mockProcess.stdout.write('{"event":"ready","wsPort":63796}\n');

    await expect(portPromise).resolves.toBe(63796);
    expect(execaMock).toHaveBeenCalledWith(
      config.binaryPath,
      expect.arrayContaining(['-ws-port', '8080', '-exit-on-stdin-close']),
      expect.objectContaining({
        stdin: 'pipe',
        stdout: 'pipe',
        stderr: 'pipe',
        buffer: false,
        cleanup: true,
      }),
    );
    expect(isGoProcessRunning()).toBe(true);
  });

  it('uses wait-on instead of spawning when configured to connect to an existing engine', async () => {
    server = createServer();
    server.listen(0, '127.0.0.1');
    await onceEvent(server, 'listening');
    const address = server.address();
    const port =
      typeof address === 'object' && address !== null ? address.port : 0;
    process.env.BISTRO_SKIP_GO_SPAWN = '1';

    await expect(
      spawnGoProcess({
        config: { ...config, wsPort: port },
        logger: logger as Logger,
      }),
    ).resolves.toBe(port);

    expect(execaMock).not.toHaveBeenCalled();
  });

  it('falls back to the configured port on legacy readiness log lines', async () => {
    const mockProcess = createMockProcess();
    execaMock.mockReturnValue(mockProcess);

    const portPromise = spawnGoProcess({ config, logger: logger as Logger });
    mockProcess.stdout.write('WebSocket server listening on :8080\n');

    await expect(portPromise).resolves.toBe(8080);
  });

  it('kills the managed process with SIGTERM', async () => {
    const mockProcess = createMockProcess();
    execaMock.mockReturnValue(mockProcess);

    const portPromise = spawnGoProcess({ config, logger: logger as Logger });
    mockProcess.stdout.write('{"event":"ready","wsPort":8080}\n');
    await portPromise;

    killGoProcess();

    expect(mockProcess.kill).toHaveBeenCalledWith('SIGTERM');
    expect(isGoProcessRunning()).toBe(false);
  });

  it('does not restart after intentional shutdown', async () => {
    const mockProcess = createMockProcess();
    const onCrash = vi.fn();
    execaMock.mockReturnValue(mockProcess);

    const portPromise = spawnGoProcess({
      config,
      logger: logger as Logger,
      onCrash,
    });
    mockProcess.stdout.write('{"event":"ready","wsPort":8080}\n');
    await portPromise;

    killGoProcess();
    await mockProcess;

    expect(onCrash).not.toHaveBeenCalled();
    expect(execaMock).toHaveBeenCalledTimes(1);
  });
});
