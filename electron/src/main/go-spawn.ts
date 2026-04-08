import { spawn, type ChildProcess } from 'child_process';
import { Logger } from 'electron-log';
import type { GoProcessConfig } from './types';

let goProcess: ChildProcess | null = null;

export function spawnGoProcess(config: GoProcessConfig, logger: Logger): void {
  if (goProcess) {
    logger.warn('Go process already running');
    return;
  }

  logger.info(`Spawning Go binary: ${config.binaryPath}`);

  goProcess = spawn(config.binaryPath, [
    `--db=${config.dbPath}`,
    `--ws-port=${config.wsPort}`,
  ]);

  goProcess.stdout?.on('data', (data) => {
    const lines = data.toString().split('\n').filter(Boolean);
    lines.forEach((line: string) => {
      logger.info(`[Go] ${line}`);
      // TODO: Parse port from stdout if needed
    });
  });

  goProcess.stderr?.on('data', (data) => {
    logger.error(`[Go stderr] ${data.toString()}`);
  });

  goProcess.on('exit', (code, signal) => {
    logger.warn(`Go process exited: code=${code}, signal=${signal}`);
    goProcess = null;
    // TODO: Emit event to renderer to show "Go crashed" error
  });

  process.on('exit', () => {
    if (goProcess) {
      goProcess.kill();
    }
  });
}

export function isGoProcessRunning(): boolean {
  return goProcess !== null && !goProcess.killed;
}

export function killGoProcess(): void {
  if (goProcess && !goProcess.killed) {
    goProcess.kill();
    goProcess = null;
  }
}
