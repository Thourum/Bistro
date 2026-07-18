import type { PR } from '../types';

export const LANES = [
  { id: 1, key: 'needs_action', label: 'NEEDS ACTION' },
  { id: 2, key: 'waiting_on_author', label: 'WAITING ON AUTHOR' },
  { id: 3, key: 'your_prs', label: 'YOUR PRS' },
  { id: 4, key: 'your_code', label: 'YOUR CODE' },
  { id: 5, key: 'mentioned', label: 'MENTIONED' },
  { id: 6, key: 'fyi', label: 'FYI' },
] as const;

export type LaneId = (typeof LANES)[number]['id'];

/** First-match-wins lane assignment. Spec: 2026-07-18 Phase 5 design §5.1. */
export function laneForPR(pr: PR, viewerLogin: string): LaneId {
  const hasViewerLogin = viewerLogin !== '';
  const isAuthor = hasViewerLogin && pr.author === viewerLogin;
  const isOpen = pr.state === 'open';

  if (hasViewerLogin) {
    const reviewed =
      pr.approved_by === viewerLogin || pr.changes_requested_by === viewerLogin;

    if (isOpen) {
      if (isAuthor && pr.review_status === 'changes_requested') return 1;
      if (pr.requested_reviewers.includes(viewerLogin)) return 1;
      if (!isAuthor && reviewed && pr.reviewed_head_stale) return 1;
    }

    if (!isAuthor && isOpen && (reviewed || pr.viewer_commented)) return 2;
    if (isAuthor) return 3;
  }

  if (pr.touches_you) return 4;
  if (pr.mentioned) return 5;
  return 6;
}
