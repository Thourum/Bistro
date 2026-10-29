import { Fault } from '@thourum/fault';
import {
  type FileViewedAck,
  fileViewedAckSchema,
  type PRsReadAck,
  prsReadAckSchema,
} from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';
import { wsClient } from './ws/connection';

const log = createLogger('readState');

/** Marks PRs read locally through the sync engine. */
export function markPRsRead(
  prIds: readonly string[],
): Promise<PRsReadAck | null> {
  return wsClient
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
    );
}

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
export function markFileViewed(
  prId: string,
  filePath: string,
  contentHash: string,
): Promise<FileViewedAck | null> {
  return wsClient
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
    );
}
