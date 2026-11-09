import { router } from '../router';
import { selectCommitIndex } from '../services/commits';
import { emitAppEvent } from './appEvents';
import {
  COMMIT_TOGGLE_VIEWED_EVENT,
  COMMIT_VIEWED_AND_NEXT_EVENT,
  commitPosition,
  shaFromCommitPath,
} from './commitNavigation';
import type { KeyActionContext } from './keybindingActions';

/** Opens the commit list for the routed PR, including Escape from a commit. */
export function openCommits(context: KeyActionContext): void {
  if (context.prId === undefined) return;
  void router.navigate({
    to: '/prs/$prId/commits',
    params: { prId: context.prId },
  });
}

function moveCommitCursor(context: KeyActionContext, delta: 1 | -1): void {
  const { commitShas, selectedCommitIndex } = context.store.state;
  selectCommitIndex(
    Math.min(
      Math.max(0, selectedCommitIndex + delta),
      Math.max(0, commitShas.length - 1),
    ),
  );
}

function openSelectedCommit(context: KeyActionContext): void {
  const { commitShas, selectedCommitIndex } = context.store.state;
  const sha = commitShas[selectedCommitIndex];
  if (context.prId === undefined || sha === undefined) return;
  void router.navigate({
    to: '/prs/$prId/commits/$sha',
    params: { prId: context.prId, sha },
    search: { file: '', side: 'right' },
  });
}

function stepCommit(
  context: KeyActionContext,
  direction: 'next' | 'prev',
): void {
  const current = shaFromCommitPath(context.location.pathname);
  if (context.prId === undefined || current === null) return;
  const position = commitPosition(context.store.state.commitShas, current);
  const sha = direction === 'next' ? position?.nextSha : position?.prevSha;
  if (sha === null || sha === undefined) return;
  void router.navigate({
    to: '/prs/$prId/commits/$sha',
    params: { prId: context.prId, sha },
    search: { file: '', side: 'right' },
  });
}

export const COMMIT_ACTIONS: Readonly<
  Record<string, (context: KeyActionContext) => void>
> = {
  'detail.commits': openCommits,
  'diff.commits': openCommits,
  'commits.down': (context) => moveCommitCursor(context, 1),
  'commits.up': (context) => moveCommitCursor(context, -1),
  'commits.open': openSelectedCommit,
  'commit.next': (context) => stepCommit(context, 'next'),
  'commit.prev': (context) => stepCommit(context, 'prev'),
  'commit.toggleViewed': () => emitAppEvent(COMMIT_TOGGLE_VIEWED_EVENT),
  'commit.viewedAndNext': () => emitAppEvent(COMMIT_VIEWED_AND_NEXT_EVENT),
};
