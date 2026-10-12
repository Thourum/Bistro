import { Fault } from '@thourum/fault';
import { useCallback } from 'react';
import { wsClient } from '../services/ws/connection';
import { type FileViewedAck, fileViewedAckSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('useMarkFileViewed');

/**
 * Marks a file viewed via `MARK_FILE_VIEWED`.
 *
 * The `contentHash` MUST be the value the engine returned for the file in
 * `DIFF_READY` — it is sent back unchanged. The engine keys viewed state on
 * `(pr_id, file_path, login, content_hash)` so a file stays viewed across a
 * rebase only while its content (and thus its hash) is unchanged. The UI does
 * not compute hashes.
 *
 * Returns the ack so callers can reconcile local state, or `null` on a
 * transport failure (the engine reports a declined write as `ok: false`).
 */
export function useMarkFileViewed(): {
  markViewed: (
    prId: string,
    filePath: string,
    contentHash: string,
  ) => Promise<FileViewedAck | null>;
} {
  const markViewed = useCallback(
    (
      prId: string,
      filePath: string,
      contentHash: string,
    ): Promise<FileViewedAck | null> =>
      wsClient
        .request('MARK_FILE_VIEWED', {
          pr_id: prId,
          file_path: filePath,
          content_hash: contentHash,
        })
        .mapErr((cause) =>
          new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
        )
        .andThen((data) => parseResponse(fileViewedAckSchema, data))
        .match(
          (data) => data,
          (message) => {
            log.warn(
              { err: message.message, filePath },
              'failed to mark file viewed',
            );
            return null;
          },
        ),
    [],
  );

  return { markViewed };
}
