import { spawn, type ChildProcess } from 'child_process';
import { Logger } from 'electron-log';
import type { GoProcessConfig } from './types';

let goProcess: ChildProcess | null = null;

const GO_READY_TIMEOUT_MS = 15_000;

/**
 * Spawns the Go sync engine binary and waits for it to signal readiness.
 * Resolves when Go prints a line containing "listening" or "started" (case-insensitive),
 * or after a timeout (proceeds anyway with a warning).
 */
export function spawnGoProcess(config: GoProcessConfig, logger: Logger): Promise<void> {
  return new Promise((resolve, reject) => {
    if (goProcess) {
      logger.warn('Go process already running');
      resolve();
      return;
    }

    const args = [
      `-ws-port`, String(config.wsPort),
      `-db-path`, config.dbPath,
      `-log-level`, config.logLevel,
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
        logger.warn('Go process did not signal readiness within timeout — proceeding anyway');
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
        reject(new Error(`Go process exited prematurely: code=${code}, signal=${signal}`));
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
