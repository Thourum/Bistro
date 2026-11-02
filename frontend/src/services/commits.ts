import { Commits } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import type {
  CommitDiffResult,
  CommitsResult,
  CommitViewedAck,
} from '../types';
import { createLogger } from '../utils/logger';
import { call, callWrite, faultReason } from './engine/call';
import { addToast } from './toasts';

const log = createLogger('commits');

export function loadCommits(prId: string): ResultAsync<CommitsResult, Fault> {
  return call(() => Commits.List({ pr_id: prId }));
}

export function loadCommitDiff(
  prId: string,
  sha: string,
): ResultAsync<CommitDiffResult, Fault> {
  return call(() => Commits.Get({ pr_id: prId, sha }));
}

/** Returns the ack, or null after toasting the reason (UI rolls back). */
export function setCommitViewed(
  prId: string,
  sha: string,
  viewed: boolean,
): Promise<CommitViewedAck | null> {
  return callWrite(() => Commits.SetViewed({ pr_id: prId, sha, viewed })).match(
    (ack) => ack,
    (fault) => {
      log.warn({ err: fault.message, sha }, 'failed to set commit viewed');
      const reason = faultReason(fault);
      addToast({
        type: 'warning',
        message: reason
          ? `Could not update viewed: ${reason}`
          : 'Could not update viewed: network or communication error',
        dismissible: true,
      });
      return null;
    },
  );
}
