import { ReadState, Viewed } from '@bindings/services';
import type { FileViewedAck, PRsReadAck } from '../types';
import { createLogger } from '../utils/logger';
import { callWrite, faultReason } from './engine/call';
import { addToast } from './toasts';

const log = createLogger('readState');

/** Marks PRs read locally through the sync engine. */
export function markPRsRead(
  prIds: readonly string[],
): Promise<PRsReadAck | null> {
  return callWrite(() => ReadState.MarkRead({ pr_ids: [...prIds] })).match(
    (data) => data,
    (fault) => {
      log.warn({ err: fault.message, prIds }, 'failed to mark PRs read');
      return null;
    },
  );
}

/**
 * Marks a file viewed using the canonical content hash from Diff.Get unchanged.
 * Returns the acknowledgement, or null after showing the declined-write reason
 * or communication warning. The UI never computes hashes or guesses success.
 */
export function markFileViewed(
  prId: string,
  filePath: string,
  contentHash: string,
): Promise<FileViewedAck | null> {
  return callWrite(() =>
    Viewed.MarkFile({
      pr_id: prId,
      file_path: filePath,
      content_hash: contentHash,
    }),
  ).match(
    (data) => data,
    (fault) => {
      log.warn({ err: fault.message, filePath }, 'failed to mark file viewed');
      const reason = faultReason(fault);
      addToast({
        type: 'warning',
        message: reason
          ? `Could not mark viewed: ${reason}`
          : 'Could not mark viewed: network or communication error',
        dismissible: true,
      });
      return null;
    },
  );
}
