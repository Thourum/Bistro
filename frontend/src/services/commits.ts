import { Commits } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { appStore } from '../store';
import type {
  CommitDiffResult,
  CommitsResult,
  CommitViewedAck,
} from '../types';
import { createLogger } from '../utils/logger';
import { call, callWrite, faultReason } from './engine/call';
import { addToast } from './toasts';

const log = createLogger('commits');

/** Clears stale route navigation without losing the list selection. */
export function clearCommitList(): void {
  appStore.setState((state) => ({ ...state, commitShas: [] }));
}

/** Seeds a direct-visit cursor, or clamps the existing list selection. */
export function setCommitList(
  shas: readonly string[],
  selectedSha?: string,
): void {
  appStore.setState((state) => ({
    ...state,
    commitShas: [...shas],
    selectedCommitIndex:
      selectedSha === undefined
        ? Math.min(
            Math.max(state.selectedCommitIndex, 0),
            Math.max(0, shas.length - 1),
          )
        : Math.max(0, shas.indexOf(selectedSha)),
  }));
}

export function selectCommitIndex(index: number): void {
  appStore.setState((state) => ({ ...state, selectedCommitIndex: index }));
}

/** Commit diffs must not inherit file navigation or display overrides. */
export function resetCommitDiffState(): void {
  appStore.setState((state) => ({
    ...state,
    diffFileKeys: [],
    diffCollapseOverrides: [],
    hideWhitespaceHunks: false,
  }));
}

export function setCommitDiffFileKeys(keys: readonly string[]): void {
  appStore.setState((state) => ({ ...state, diffFileKeys: [...keys] }));
}

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
