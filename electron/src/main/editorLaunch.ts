import { type ChildProcess, spawn } from 'node:child_process';
import type { Logger } from 'electron-log';
import {
  errAsync,
  fromPromise,
  fromThrowable,
  type ResultAsync,
} from 'neverthrow';

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
): ResultAsync<void, string> {
  const childResult = fromThrowable(
    () =>
      spawnProcess(command, args, {
        detached: true,
        stdio: 'ignore',
      }),
    errorMessage,
  )();

  if (childResult.isErr()) {
    logger.error(`open-in-editor error: ${childResult.error}`);
    return errAsync(childResult.error);
  }

  const child = childResult.value;
  return fromPromise(
    new Promise<void>((resolve, reject) => {
      child.once('error', reject);
      child.once('spawn', () => {
        child.unref();
        resolve();
      });
    }),
    (error) => {
      const message = errorMessage(error);
      logger.error(`open-in-editor spawn error: ${message}`);
      return message;
    },
  );
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
