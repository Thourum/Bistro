import { readFile } from 'node:fs/promises';
import { homedir } from 'node:os';
import { dirname } from 'node:path';
import {
  errAsync,
  Fault,
  fromPromise,
  okAsync,
  type ResultAsync,
  ServiceError,
} from '@thourum/fault';
import { app, ipcMain, shell } from 'electron';
import { type EngineStatus, engineLogRequestSchema } from '../types/engine';
import { buildCrashIssueUrl, REPORT_LOG_LINES } from './crashReport';
import { getStderrTail } from './go-spawn';

interface EngineIPCOptions {
  readonly logFile: string;
  readonly getStatus: () => EngineStatus;
  readonly restart: () => ResultAsync<void, Fault>;
}

function engineFault(cause: unknown): Fault {
  return new Fault(cause instanceof Error ? cause.message : String(cause))
    .withTag('ENGINE_ERROR')
    .withCause(cause);
}

function readLogTail(
  logFile: string,
  count: number,
): ResultAsync<readonly string[], Fault> {
  return fromPromise(readFile(logFile, 'utf8'), (cause) => {
    if (cause instanceof Error && 'code' in cause && cause.code === 'ENOENT') {
      return ServiceError(
        'LOG_MISSING',
        'Engine log file not present.',
      ).withCause(cause);
    }
    return engineFault(cause);
  })
    .orElse((fault) =>
      fault.tag === 'LOG_MISSING' ? okAsync('') : errAsync(fault),
    )
    .map((text) => {
      const lines = text.split(/\r?\n/).filter((line) => line.trim() !== '');
      return (lines.length > 0 ? lines : getStderrTail()).slice(-count);
    });
}

export function registerEngineIPC(options: EngineIPCOptions): void {
  ipcMain.handle('engine:get-status', () => ({ ok: options.getStatus() }));
  ipcMain.handle('app:get-version', () => ({ ok: app.getVersion() }));
  ipcMain.handle('engine:restart', () => {
    const status = options.getStatus();
    if (!status.isSpawned || status.state !== 'crashed')
      return { err: 'This engine cannot be restarted here.' };
    return options.restart().match(
      () => ({ ok: true }),
      (fault) => ({ err: fault.message }),
    );
  });
  ipcMain.handle('engine:log-tail', (_event, input: unknown) => {
    const parsed = engineLogRequestSchema.safeParse(input);
    if (!parsed.success)
      return { err: 'lines must be an integer from 1 to 60.' };
    return readLogTail(options.logFile, parsed.data.lines).match(
      (lines) => ({ ok: lines }),
      (fault) => ({ err: fault.message }),
    );
  });
  ipcMain.handle('engine:open-log-folder', () =>
    fromPromise(shell.openPath(dirname(options.logFile)), engineFault)
      .andThen((message) =>
        message === ''
          ? okAsync(true)
          : errAsync(ServiceError('ENGINE_ERROR', message)),
      )
      .match(
        (value) => ({ ok: value }),
        (fault) => ({ err: fault.message }),
      ),
  );
  ipcMain.handle('engine:report-crash', () => {
    const status = options.getStatus();
    if (status.state !== 'crashed')
      return { err: 'Crash reporting is available after the engine stops.' };
    return readLogTail(options.logFile, REPORT_LOG_LINES)
      .map((logs) =>
        buildCrashIssueUrl({
          version: app.getVersion(),
          platform: process.platform,
          arch: process.arch,
          code: status.code,
          signal: status.signal,
          home: homedir(),
          logs,
        }),
      )
      .andThen((url) => fromPromise(shell.openExternal(url), engineFault))
      .match(
        () => ({ ok: true }),
        (fault) => ({ err: fault.message }),
      );
  });
}
