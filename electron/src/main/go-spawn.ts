import { createRequire } from 'node:module';
import type { Logger } from 'electron-log';
import { execa, type ResultPromise } from 'execa';
import { fromPromise, fromThrowable, type ResultAsync } from 'neverthrow';
import type { GoProcessConfig } from './types';

const require = createRequire(import.meta.url);

type WaitOnOptions = {
  resources: string[];
  timeout: number;
  interval?: number;
  tcpTimeout?: number;
  window?: number;
};

const waitOn = require('wait-on') as (options: WaitOnOptions) => Promise<void>;

function waitForPort(
  port: number,
  timeoutMs: number,
): ResultAsync<void, Error> {
  return fromPromise(
    waitOn({
      resources: [`tcp:127.0.0.1:${port}`],
      timeout: timeoutMs,
      interval: 250,
      tcpTimeout: 300,
      window: 0,
    }),
    () =>
      new Error(`Timed out waiting for external Go process on port ${port}`),
  );
}

let goProcess: ResultPromise | null = null;

const GO_READY_TIMEOUT_MS = 15_000;
const MAX_RESTARTS = 3;
const RESTART_DELAY_MS = 2000;
const FALLBACK_EXTERNAL_PORT = 8080;

let restartCount = 0;
let lastKnownPort: number | null = null;
let exitHandlerRegistered = false;
let intentionalShutdownCount = 0;

/**
 * Parses the machine-readable ready signal the Go process prints to stdout:
 * `{"event":"ready","wsPort":63796}`. Returns the port, or null when the
 * line is not a valid ready signal.
 */
export function parseReadyLine(line: string): number | null {
  const trimmed = line.trim();
  if (!trimmed.startsWith('{')) {
    return null;
  }

  const parsedResult = fromThrowable(
    () => JSON.parse(trimmed) as unknown,
    () => null,
  )();

  if (parsedResult.isErr()) {
    return null;
  }

  const parsed = parsedResult.value;
  if (typeof parsed !== 'object' || parsed === null) {
    return null;
  }
  const record = parsed as Record<string, unknown>;
  if (record.event !== 'ready') {
    return null;
  }
  const port = record.wsPort;
  if (
    typeof port !== 'number' ||
    !Number.isInteger(port) ||
    port < 1 ||
    port > 65535
  ) {
    return null;
  }
  return port;
}

export interface GoSpawnOptions {
  config: GoProcessConfig;
  logger: Logger;
  onCrash?: (code: number | null, signal: string | null) => void;
  maxRestarts?: number;
}

const createGoArgs = (config: GoProcessConfig): string[] => {
  const args = [
    `-ws-port`,
    String(config.wsPort),
    `-db-path`,
    config.dbPath,
    `-log-level`,
    config.logLevel,
    `-replay-queue=${config.replayQueue}`,
    `-sync-interval`,
    `${config.syncInterval}s`,
    `-auto-sync=${config.autoSync}`,
    // Go exits when its stdin pipe closes, so it can never outlive us
    // even if this process is SIGKILLed (see stdio below).
    `-exit-on-stdin-close`,
  ];

  return config.logFile ? [...args, `-log-file`, config.logFile] : args;
};

const normalizeExitCode = (exitCode: number | undefined): number | null =>
  exitCode ?? null;

const normalizeSignal = (signal: string | undefined): string | null =>
  signal ?? null;

const getExitDetails = (
  error: unknown,
): { code: number | null; signal: string | null; message: string } => {
  if (error instanceof Error) {
    const record = error as Error & {
      exitCode?: number;
      signal?: unknown;
      shortMessage?: string;
    };
    return {
      code: normalizeExitCode(record.exitCode),
      signal:
        typeof record.signal === 'string'
          ? normalizeSignal(record.signal)
          : null,
      message: record.shortMessage ?? record.message,
    };
  }
  return { code: null, signal: null, message: String(error) };
};

const handleProcessExit = (
  code: number | null,
  signal: string | null,
  logger: Logger,
  onCrash: GoSpawnOptions['onCrash'],
  maxRestarts: number,
  options: GoSpawnOptions,
  wasStartupResolved: boolean,
): void => {
  if (!wasStartupResolved) {
    return;
  }

  if (intentionalShutdownCount > 0) {
    intentionalShutdownCount -= 1;
    return;
  }

  if (onCrash != null) {
    onCrash(code, signal);
  }

  // Auto-restart if under limit. Reuse the last known port so the main
  // process and renderer reconnect logic keep working after a crash.
  if (restartCount < maxRestarts) {
    restartCount += 1;
    logger.info(
      `Auto-restarting Go process (attempt ${restartCount}/${maxRestarts}) in ${RESTART_DELAY_MS}ms`,
    );
    const restartOptions: GoSpawnOptions =
      lastKnownPort !== null
        ? {
            ...options,
            config: { ...options.config, wsPort: lastKnownPort },
          }
        : options;
    setTimeout(() => {
      spawnGoProcess(restartOptions).catch((err: unknown) => {
        const message = err instanceof Error ? err.message : String(err);
        logger.error(`Failed to restart Go process: ${message}`);
      });
    }, RESTART_DELAY_MS);
    return;
  }

  logger.error(`Go process crashed ${restartCount} times — not restarting`);
};

const attachProcessLifecycle = (
  processPromise: ResultPromise,
  options: GoSpawnOptions,
  wasStartupResolved: () => boolean,
  onStartupExit: (error: Error) => void,
): void => {
  const { logger, onCrash, maxRestarts = MAX_RESTARTS } = options;

  processPromise.then(
    (result) => {
      logger.warn(
        `Go process exited: code=${result.exitCode ?? null}, signal=${result.signal ?? null}`,
      );
      goProcess = null;
      const startupResolved = wasStartupResolved();
      if (!startupResolved) {
        onStartupExit(
          new Error(
            `Go process exited prematurely: code=${result.exitCode ?? null}, signal=${result.signal ?? null}`,
          ),
        );
        return;
      }
      handleProcessExit(
        normalizeExitCode(result.exitCode),
        normalizeSignal(result.signal),
        logger,
        onCrash,
        maxRestarts,
        options,
        startupResolved,
      );
    },
    (error: unknown) => {
      const exitDetails = getExitDetails(error);
      logger.warn(
        `Go process exited: code=${exitDetails.code}, signal=${exitDetails.signal}`,
      );
      goProcess = null;
      const startupResolved = wasStartupResolved();
      if (!startupResolved) {
        onStartupExit(
          new Error(
            `Go process exited prematurely: code=${exitDetails.code}, signal=${exitDetails.signal}`,
          ),
        );
        return;
      }
      handleProcessExit(
        exitDetails.code,
        exitDetails.signal,
        logger,
        onCrash,
        maxRestarts,
        options,
        startupResolved,
      );
    },
  );
};

const registerExitHandler = (): void => {
  if (exitHandlerRegistered) {
    return;
  }
  exitHandlerRegistered = true;
  process.on('exit', () => {
    killGoProcess();
  });
};

/**
 * Spawns the Go sync engine binary and waits for it to signal readiness.
 * Resolves with the actual WebSocket port once Go prints its structured
 * ready line (`{"event":"ready","wsPort":N}`). Falls back to legacy log-line
 * sniffing for older binaries when a fixed port was configured.
 *
 * Pass `config.wsPort = 0` to let Go pick a random available port.
 */
export function spawnGoProcess(options: GoSpawnOptions): Promise<number> {
  const { config, logger } = options;

  // When BISTRO_SKIP_GO_SPAWN=1, assume Go is already running externally (e.g. under Delve for debugging).
  if (process.env.BISTRO_SKIP_GO_SPAWN === '1') {
    const externalPort =
      config.wsPort > 0 ? config.wsPort : FALLBACK_EXTERNAL_PORT;
    logger.info(
      'BISTRO_SKIP_GO_SPAWN=1 — waiting for external Go process on port ' +
        externalPort,
    );
    return waitForPort(externalPort, GO_READY_TIMEOUT_MS)
      .map(() => {
        lastKnownPort = externalPort;
        return externalPort;
      })
      .match(
        (port) => ({ ok: true, port }) as const,
        (error) => ({ ok: false, error }) as const,
      )
      .then((result) =>
        result.ok ? Promise.resolve(result.port) : Promise.reject(result.error),
      );
  }

  if (goProcess) {
    logger.warn('Go process already running');
    return Promise.resolve(lastKnownPort ?? config.wsPort);
  }

  const args = createGoArgs(config);
  logger.info(`Spawning Go binary: ${config.binaryPath} ${args.join(' ')}`);

  const processResult = fromThrowable(
    () =>
      execa(config.binaryPath, args, {
        // stdin must be a pipe: Go watches it for EOF to detect parent death.
        stdin: 'pipe',
        stdout: 'pipe',
        stderr: 'pipe',
        buffer: false,
        cleanup: true,
        forceKillAfterDelay: 5000,
      }),
    (error) => (error instanceof Error ? error : new Error(String(error))),
  )();

  if (processResult.isErr()) {
    const message = processResult.error.message;
    logger.error(`Failed to spawn Go process: ${message}`);
    return Promise.reject(new Error(`Failed to spawn Go process: ${message}`));
  }

  const spawnedProcess = processResult.value;
  goProcess = spawnedProcess;
  registerExitHandler();

  return new Promise((resolve, reject) => {
    let resolved = false;

    const readyTimeout = setTimeout(() => {
      if (resolved) {
        return;
      }
      resolved = true;
      if (config.wsPort > 0) {
        logger.warn(
          'Go process did not signal readiness within timeout — proceeding with configured port',
        );
        resolve(config.wsPort);
        return;
      }
      reject(
        new Error(
          'Go process did not emit a ready signal — cannot determine WebSocket port',
        ),
      );
    }, GO_READY_TIMEOUT_MS);

    const markReady = (port: number, source: string): void => {
      if (resolved) {
        return;
      }
      resolved = true;
      clearTimeout(readyTimeout);
      lastKnownPort = port;
      logger.info(`Go sync engine is ready on port ${port} (${source})`);
      resolve(port);
    };

    spawnedProcess.stdout?.on('data', (data: Buffer) => {
      const lines = data.toString().split('\n').filter(Boolean);
      for (const line of lines) {
        logger.info(`[go] ${line}`);

        const readyPort = parseReadyLine(line);
        if (readyPort !== null) {
          markReady(readyPort, 'ready signal');
          continue;
        }
        // Legacy fallback: older binaries only log "WebSocket server listening …"
        if (config.wsPort > 0 && /listen|started|serving/i.test(line)) {
          markReady(config.wsPort, 'log line');
        }
      }
    });

    spawnedProcess.stderr?.on('data', (data: Buffer) => {
      const lines = data.toString().split('\n').filter(Boolean);
      for (const line of lines) {
        logger.warn(`[go:stderr] ${line}`);

        // Some Go programs log to stderr; also check for readiness signals there
        if (config.wsPort > 0 && /listen|started|serving/i.test(line)) {
          markReady(config.wsPort, 'stderr log line');
        }
      }
    });

    spawnedProcess.once('spawn', () => {
      logger.info('Go process spawned');
    });

    spawnedProcess.once('error', (error: Error) => {
      logger.error(`Go process error: ${error.message}`);
      if (!resolved) {
        resolved = true;
        clearTimeout(readyTimeout);
        reject(new Error(`Go process error: ${error.message}`));
      }
    });

    attachProcessLifecycle(
      spawnedProcess,
      options,
      () => resolved,
      (error) => {
        if (!resolved) {
          resolved = true;
          clearTimeout(readyTimeout);
          reject(error);
        }
      },
    );
  });
}

export function isGoProcessRunning(): boolean {
  return goProcess !== null && !goProcess.killed;
}

export function killGoProcess(): void {
  if (goProcess && !goProcess.killed) {
    intentionalShutdownCount += 1;
    goProcess.kill('SIGTERM');
    goProcess = null;
  }
}
