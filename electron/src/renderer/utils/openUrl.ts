import { err, Fault, ok, ResultAsync, ServiceError } from '@thourum/fault';
import { guardBenchWrite } from './benchSafety';

/**
 * Open a URL in the user's real browser.
 *
 * In Electron this goes through the main process (`openExternal`); under
 * `pnpm dev:web` it falls back to `window.open`.
 */
export function openUrlInBrowser(url: string): ResultAsync<void, Fault> {
  const permitted = guardBenchWrite('open-external');
  if (permitted.isErr()) return new ResultAsync(Promise.resolve(permitted));
  if (window.electron?.openExternal !== undefined) {
    return ResultAsync.fromPromise(window.electron.openExternal(url), (cause) =>
      new Fault(cause instanceof Error ? cause.message : String(cause))
        .withTag('OPEN_URL_ERROR')
        .withCause(cause),
    ).andThen((response) =>
      'err' in response
        ? err(ServiceError('OPEN_URL_ERROR', response.err))
        : ok(undefined),
    );
  }

  return ResultAsync.fromPromise(
    (async () => {
      window.open(url, '_blank', 'noopener,noreferrer');
    })(),
    (cause) =>
      new Fault(cause instanceof Error ? cause.message : String(cause))
        .withTag('OPEN_URL_ERROR')
        .withCause(cause),
  );
}
