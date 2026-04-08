import { useCallback } from 'react';
import { ResultAsync } from 'neverthrow';
import type { DBQueryRequest } from '../../types/ipc';

export type QueryResult<T> = ResultAsync<T[], string>;

function isElectronAvailable(): boolean {
  return typeof window !== 'undefined' && 'electron' in window && window.electron !== undefined;
}

export function useSQLiteQuery() {
  const query = useCallback(
    <T extends Record<string, unknown>>(
      sql: string,
      params?: unknown[]
    ): QueryResult<T> => {
      if (!isElectronAvailable()) {
        return ResultAsync.fromPromise(
          Promise.resolve([] as T[]),
          () => 'SQLite not available in browser mode'
        );
      }

      return ResultAsync.fromPromise(
        (async () => {
          const request: DBQueryRequest = params !== undefined ? { sql, params } : { sql };
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          const response = await (globalThis as any).window.electron.db(request);

          if ('err' in response) {
            throw new Error(response.err);
          }

          return response.ok as T[];
        })(),
        (error) => {
          return error instanceof Error ? error.message : 'Unknown error';
        }
      );
    },
    []
  );

  return { query };
}
