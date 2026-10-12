import { type ChildProcess, spawn } from 'node:child_process';
import {
  errAsync,
  Fault,
  fromPromise,
  fromThrowable,
  type ResultAsync,
} from '@thourum/fault';
import type { Logger } from 'electron-log';

type SpawnEditor = (
  command: string,
  args: readonly string[],
  options: { detached: true; stdio: 'ignore' },
) => ChildProcess;

const spawnEditor: SpawnEditor = (command, args, options) =>
  spawn(command, args, options);

/**
 * Starts an editor as a fully detached child and resolves only after Node emits
 * `spawn`. `child_process.spawn()` reports missing executables asynchronously,
 * so returning immediately would falsely tell the renderer that the open
 * succeeded on ENOENT.
 */
export function launchEditor(
  command: string,
  args: readonly string[],
  logger: Logger,
  spawnProcess: SpawnEditor = spawnEditor,
): ResultAsync<void, Fault> {
  const childResult = fromThrowable(
    () =>
      spawnProcess(command, args, {
        detached: true,
        stdio: 'ignore',
      }),
    (cause) =>
      new Fault(errorMessage(cause))
        .withTag('EDITOR_LAUNCH_ERROR')
        .withCause(cause),
  )();

  if (childResult.isErr()) {
    logger.error(`open-in-editor error: ${childResult.error.message}`);
    return errAsync(childResult.error);
  }

  const child = childResult.value;
  return fromPromise(
    new Promise<void>((resolve, reject) => {
      // Detach whichever listener didn't fire so the closures don't outlive
      // the spawn handshake on a long-running editor process.
      const onError = (error: Error) => {
        child.off('spawn', onSpawn);
        reject(error);
      };
      const onSpawn = () => {
        child.off('error', onError);
        child.unref();
        resolve();
      };
      child.once('error', onError);
      child.once('spawn', onSpawn);
    }),
    (error) => {
      const message = errorMessage(error);
      logger.error(`open-in-editor spawn error: ${message}`);
      return new Fault(message).withTag('EDITOR_LAUNCH_ERROR').withCause(error);
    },
  );
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
