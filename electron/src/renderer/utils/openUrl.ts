import { err, ok, ResultAsync } from 'neverthrow';

type OpenUrlError = { type: 'open-url'; message: string };

/**
 * Open a URL in the user's real browser.
 *
 * In Electron this goes through the main process (`openExternal`); under
 * `pnpm dev:web` it falls back to `window.open`.
 */
export function openUrlInBrowser(url: string): ResultAsync<void, OpenUrlError> {
  if (window.electron?.openExternal !== undefined) {
    return ResultAsync.fromPromise(
      window.electron.openExternal(url),
      (cause) => ({
        type: 'open-url' as const,
        message: cause instanceof Error ? cause.message : String(cause),
      }),
    ).andThen((response) =>
      'err' in response
        ? err({ type: 'open-url' as const, message: response.err })
        : ok(undefined),
    );
  }

  return ResultAsync.fromPromise(
    (async () => {
      window.open(url, '_blank', 'noopener,noreferrer');
    })(),
    (cause): OpenUrlError => ({
      type: 'open-url',
      message: cause instanceof Error ? cause.message : String(cause),
    }),
  );
}
