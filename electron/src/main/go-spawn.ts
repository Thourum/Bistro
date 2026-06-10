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
const FALLBACK_EXTERNAL_PORT = 8080;

let restartCount = 0;
let lastKnownPort: number | null = null;

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
  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return null;
  }
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

/**
 * Spawns the Go sync engine binary and waits for it to signal readiness.
 * Resolves with the actual WebSocket port once Go prints its structured
 * ready line (`{"event":"ready","wsPort":N}`). Falls back to legacy log-line
 * sniffing for older binaries when a fixed port was configured.
 *
 * Pass `config.wsPort = 0` to let Go pick a random available port.
 */
export function spawnGoProcess(options: GoSpawnOptions): Promise<number> {
  const { config, logger, onCrash, maxRestarts = MAX_RESTARTS } = options;
  return new Promise((resolve, reject) => {
    // When BISTRO_SKIP_GO_SPAWN=1, assume Go is already running externally (e.g. under Delve for debugging).
    if (process.env.BISTRO_SKIP_GO_SPAWN === '1') {
      const externalPort =
        config.wsPort > 0 ? config.wsPort : FALLBACK_EXTERNAL_PORT;
      logger.info(
        'BISTRO_SKIP_GO_SPAWN=1 — waiting for external Go process on port ' +
          externalPort,
      );
      waitForPort(externalPort, GO_READY_TIMEOUT_MS)
        .then(() => {
          lastKnownPort = externalPort;
          resolve(externalPort);
        })
        .catch(reject);
      return;
    }

    if (goProcess) {
      logger.warn('Go process already running');
      resolve(lastKnownPort ?? config.wsPort);
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
        if (config.wsPort > 0) {
          logger.warn(
            'Go process did not signal readiness within timeout — proceeding with configured port',
          );
          resolve(config.wsPort);
        } else {
          reject(
            new Error(
              'Go process did not emit a ready signal — cannot determine WebSocket port',
            ),
          );
        }
      }
    }, GO_READY_TIMEOUT_MS);

    const markReady = (port: number, source: string): void => {
      resolved = true;
      clearTimeout(readyTimeout);
      lastKnownPort = port;
      logger.info(`Go sync engine is ready on port ${port} (${source})`);
      resolve(port);
    };

    goProcess.stdout?.on('data', (data: Buffer) => {
      const lines = data.toString().split('\n').filter(Boolean);
      for (const line of lines) {
        logger.info(`[go] ${line}`);

        if (resolved) {
          continue;
        }
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

    goProcess.stderr?.on('data', (data: Buffer) => {
      const lines = data.toString().split('\n').filter(Boolean);
      for (const line of lines) {
        logger.warn(`[go:stderr] ${line}`);

        // Some Go programs log to stderr; also check for readiness signals there
        if (
          !resolved &&
          config.wsPort > 0 &&
          /listen|started|serving/i.test(line)
        ) {
          markReady(config.wsPort, 'stderr log line');
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
