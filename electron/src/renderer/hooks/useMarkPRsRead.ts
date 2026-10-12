import { Fault } from '@thourum/fault';
import { useCallback } from 'react';
import { wsClient } from '../services/ws/connection';
import { type PRsReadAck, prsReadAckSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('useMarkPRsRead');

/** Marks PRs read locally through the sync engine. */
export function useMarkPRsRead(): {
  markPRsRead: (prIds: readonly string[]) => Promise<PRsReadAck | null>;
} {
  const markPRsRead = useCallback(
    (prIds: readonly string[]): Promise<PRsReadAck | null> =>
      wsClient
        .request('MARK_PRS_READ', { pr_ids: [...prIds] })
        .mapErr((cause) =>
          new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
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
