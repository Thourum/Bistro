import { open } from 'node:fs/promises';
import { homedir } from 'node:os';
import { dirname } from 'node:path';
import {
  errAsync,
  Fault,
  fromPromise,
  okAsync,
  type Result,
  ResultAsync,
  ServiceError,
} from '@thourum/fault';
import { app, shell } from 'electron';
import { type EngineStatus, engineLogRequestSchema } from '../types/engine';
import { registerMainIpc } from './benchIpc';
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

// Bound allocations even when years of append-only logs precede the tail.
const MAX_LOG_TAIL_BYTES = 64 * 1024;

function readLogSuffix(logFile: string): ResultAsync<string, Fault> {
  return fromPromise(open(logFile, 'r'), (cause) => {
    if (cause instanceof Error && 'code' in cause && cause.code === 'ENOENT') {
      return ServiceError(
        'LOG_MISSING',
        'Engine log file not present.',
      ).withCause(cause);
    }
    return engineFault(cause);
  })
    .andThen((file) => {
      const suffix = fromPromise(file.stat(), engineFault).andThen(
        ({ size }) => {
          const start = Math.max(0, size - MAX_LOG_TAIL_BYTES);
          const buffer = Buffer.alloc(size - start);
          return fromPromise(
            file.read(buffer, 0, buffer.length, start),
            engineFault,
          ).map(({ bytesRead }) => {
            const text = buffer.subarray(0, bytesRead).toString('utf8');
            // Drop the cut line (including any partial UTF-8 character).
            if (start === 0) return text;
            const firstNewline = text.indexOf('\n');
            return firstNewline < 0 ? '' : text.slice(firstNewline + 1);
          });
        },
      );
      return ResultAsync.fromSafePromise(
        suffix.then<Result<string, Fault>, never>((result) => result),
      ).andThen((result) =>
        fromPromise(file.close(), engineFault).andThen(() => result),
      );
    })
    .orElse((fault) =>
      fault.tag === 'LOG_MISSING' ? okAsync('') : errAsync(fault),
    );
}

function readLogTail(
  logFile: string,
  count: number,
): ResultAsync<readonly string[], Fault> {
  return readLogSuffix(logFile).map((text) => {
    const lines = text.split(/\r?\n/).filter((line) => line.trim() !== '');
    return [...lines, ...getStderrTail()].slice(-count);
  });
}

export function registerEngineIPC(options: EngineIPCOptions): void {
  registerMainIpc('engine:get-status', () => ({ ok: options.getStatus() }));
  registerMainIpc('app:get-version', () => ({ ok: app.getVersion() }));
  registerMainIpc('engine:restart', () => {
    const status = options.getStatus();
    if (!status.isSpawned || status.state !== 'crashed')
      return { err: 'This engine cannot be restarted here.' };
    return options.restart().match(
      () => ({ ok: true }),
      (fault) => ({ err: fault.message }),
    );
  });
  registerMainIpc('engine:log-tail', (_event, input: unknown) => {
    const parsed = engineLogRequestSchema.safeParse(input);
    if (!parsed.success)
      return { err: 'lines must be an integer from 1 to 60.' };
    return readLogTail(options.logFile, parsed.data.lines).match(
      (lines) => ({ ok: lines }),
      (fault) => ({ err: fault.message }),
    );
  });
  registerMainIpc('engine:open-log-folder', () =>
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
  registerMainIpc('engine:report-crash', () => {
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
