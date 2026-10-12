import { Fault, fromPromise } from '@thourum/fault';
import { useCallback, useRef } from 'react';
import { type PRsReadAck, prsReadAckSchema } from '../types';
import { parseResponse } from '../utils/error';
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
        (cause) =>
          new Fault(cause instanceof Error ? cause : 'Unknown error')
            .withTag('NETWORK_ERROR')
            .withCause(cause),
      )
        .andThen((data) => parseResponse(prsReadAckSchema, data))
        .match(
          (data) => data,
          (err) => {
            log.warn({ err: err.message, prIds }, 'failed to mark PRs read');
            return null;
          },
        ),
    [],
  );

  return { markPRsRead };
}
