import { useCallback } from 'react';
import { ResultAsync } from 'neverthrow';
import type { DBQueryRequest } from '../../types/ipc';

export type QueryResult<T> = ResultAsync<T[], string>;

export function useSQLiteQuery() {
  const query = useCallback(
    <T extends Record<string, unknown>>(
      sql: string,
      params?: unknown[]
    ): QueryResult<T> => {
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
