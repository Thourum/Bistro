import { fromPromise } from 'neverthrow';
import { useCallback, useRef } from 'react';
import type { PRsReadAck } from '../types';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('useMarkPRsRead');

/** Marks PRs read locally through the sync engine. */
export function useMarkPRsRead(): {
  markPRsRead: (prIds: readonly string[]) => Promise<PRsReadAck | null>;
} {
  const { request } = useWebSocket();
  const requestRef = useRef(request);
  requestRef.current = request;

  const markPRsRead = useCallback(
    (prIds: readonly string[]): Promise<PRsReadAck | null> =>
      fromPromise(
        requestRef.current('MARK_PRS_READ', { pr_ids: [...prIds] }),
        (err) => err,
      ).match(
        (data) => data as PRsReadAck,
        (err) => {
          log.warn({ err, prIds }, 'failed to mark PRs read');
          return null;
        },
      ),
    [],
  );

  return { markPRsRead };
}
