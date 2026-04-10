import { type ChildProcess, spawn } from 'node:child_process';
import { createConnection } from 'node:net';
import type { Logger } from 'electron-log';
import type { GoProcessConfig } from './types';

function waitForPort(port: number, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  return new Promise((resolve, reject) => {
    const attempt = () => {
      const socket = createConnection({ host: '127.0.0.1', port });
      socket.once('connect', () => {
        socket.destroy();
        resolve();
      });
      socket.once('error', () => {
        socket.destroy();
        if (Date.now() >= deadline) {
          reject(
            new Error(
              `Timed out waiting for external Go process on port ${port}`,
            ),
          );
          return;
        }
        setTimeout(attempt, 250);
      });
    };
    attempt();
  });
}

let goProcess: ChildProcess | null = null;

const GO_READY_TIMEOUT_MS = 15_000;
const MAX_RESTARTS = 3;
const RESTART_DELAY_MS = 2000;

let restartCount = 0;

export interface GoSpawnOptions {
  config: GoProcessConfig;
  logger: Logger;
  onCrash?: (code: number | null, signal: string | null) => void;
  maxRestarts?: number;
}

/**
 * Spawns the Go sync engine binary and waits for it to signal readiness.
 * Resolves when Go prints a line containing "listening" or "started" (case-insensitive),
 * or after a timeout (proceeds anyway with a warning).
 */
export function spawnGoProcess(options: GoSpawnOptions): Promise<void> {
  const { config, logger, onCrash, maxRestarts = MAX_RESTARTS } = options;
  return new Promise((resolve, reject) => {
    // When BISTRO_SKIP_GO_SPAWN=1, assume Go is already running externally (e.g. under Delve for debugging).
    if (process.env.BISTRO_SKIP_GO_SPAWN === '1') {
      logger.info(
        'BISTRO_SKIP_GO_SPAWN=1 — waiting for external Go process on port ' +
          config.wsPort,
      );
      waitForPort(config.wsPort, GO_READY_TIMEOUT_MS)
        .then(resolve)
        .catch(reject);
      return;
    }

    if (goProcess) {
      logger.warn('Go process already running');
      resolve();
      return;
    }

    const args = [
      `-ws-port`,
      String(config.wsPort),
      `-db-path`,
      config.dbPath,
      `-log-level`,
      config.logLevel,
      `-replay-queue=${config.replayQueue}`,
    ];

    if (config.logFile) {
      args.push(`-log-file`, config.logFile);
    }

    logger.info(`Spawning Go binary: ${config.binaryPath} ${args.join(' ')}`);

    try {
      goProcess = spawn(config.binaryPath, args, {
        stdio: ['ignore', 'pipe', 'pipe'],
      });
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      logger.error(`Failed to spawn Go process: ${message}`);
      reject(new Error(`Failed to spawn Go process: ${message}`));
      return;
    }

    let resolved = false;

    const readyTimeout = setTimeout(() => {
      if (!resolved) {
        resolved = true;
        logger.warn(
          'Go process did not signal readiness within timeout — proceeding anyway',
        );
        resolve();
      }
    }, GO_READY_TIMEOUT_MS);

    goProcess.stdout?.on('data', (data: Buffer) => {
      const lines = data.toString().split('\n').filter(Boolean);
      for (const line of lines) {
        logger.info(`[go] ${line}`);

        // Detect readiness: Go logs "WebSocket server listening on :8080" or similar
        if (!resolved && /listen|started|serving/i.test(line)) {
          resolved = true;
          clearTimeout(readyTimeout);
          logger.info('Go sync engine is ready');
          resolve();
        }
      }
    });

    goProcess.stderr?.on('data', (data: Buffer) => {
      const lines = data.toString().split('\n').filter(Boolean);
      for (const line of lines) {
        logger.warn(`[go:stderr] ${line}`);

        // Some Go programs log to stderr; also check for readiness signals there
        if (!resolved && /listen|started|serving/i.test(line)) {
          resolved = true;
          clearTimeout(readyTimeout);
          logger.info('Go sync engine is ready (via stderr)');
          resolve();
        }
      }
    });

    goProcess.on('error', (error) => {
      logger.error(`Go process error: ${error.message}`);
      if (!resolved) {
        resolved = true;
        clearTimeout(readyTimeout);
        reject(new Error(`Go process error: ${error.message}`));
      }
    });

    goProcess.on('exit', (code, signal) => {
      logger.warn(`Go process exited: code=${code}, signal=${signal}`);
      goProcess = null;

      if (!resolved) {
        resolved = true;
        clearTimeout(readyTimeout);
        reject(
          new Error(
            `Go process exited prematurely: code=${code}, signal=${signal}`,
          ),
        );
        return;
      }

      // Process exited after initial startup — this is a crash
      if (onCrash != null) {
        onCrash(code, signal);
      }

      // Auto-restart if under limit
      if (restartCount < maxRestarts) {
        restartCount += 1;
        logger.info(
          `Auto-restarting Go process (attempt ${restartCount}/${maxRestarts}) in ${RESTART_DELAY_MS}ms`,
        );
        setTimeout(() => {
          spawnGoProcess(options).catch((err: unknown) => {
            const message = err instanceof Error ? err.message : String(err);
            logger.error(`Failed to restart Go process: ${message}`);
          });
        }, RESTART_DELAY_MS);
      } else {
        logger.error(
          `Go process crashed ${restartCount} times — not restarting`,
        );
      }
    });

    // Ensure Go is killed when Electron exits
    process.on('exit', () => {
      if (goProcess && !goProcess.killed) {
        goProcess.kill();
      }
    });
  });
}

export function isGoProcessRunning(): boolean {
  return goProcess !== null && !goProcess.killed;
}

export function killGoProcess(): void {
  if (goProcess && !goProcess.killed) {
    goProcess.kill('SIGTERM');
    goProcess = null;
  }
}
