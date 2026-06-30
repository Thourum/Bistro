import type { PR } from '../types';

export type TriageLane = 'needs_action' | 'waiting_author' | 'fyi';

export interface TriageCounts {
  needsAction: number;
  waitingAuthor: number;
  fyi: number;
}

export function prBrowserUrl(pr: PR): string | null {
  if (pr.provider !== 'github') return null;
  return `https://github.com/${pr.owner}/${pr.repo}/pull/${pr.number}`;
}

/**
 * Classifies a PR relative to the current user (roadmap 1.6):
 * - my review is requested (and the provider clears that flag once I review)
 *   → needs_action
 * - I authored it and changes were requested → needs_action
 * - approved, waiting for merge → waiting_author
 * - drafts and everything else → fyi
 */
export function classifyPR(pr: PR, username: string): TriageLane {
  if (pr.draft === 1) {
    return 'fyi';
  }
  if (username !== '') {
    if ((pr.requested_reviewers ?? []).includes(username)) {
      return 'needs_action';
    }
    if (pr.author === username && pr.review_status === 'changes_requested') {
      return 'needs_action';
    }
  }
  if (pr.review_status === 'approved') {
    return 'waiting_author';
  }
  return 'fyi';
}

export function countTriageLanes(
  prs: readonly PR[],
  username: string,
): TriageCounts {
  return prs.reduce<TriageCounts>(
    (counts, pr) => {
      switch (classifyPR(pr, username)) {
        case 'needs_action':
          return { ...counts, needsAction: counts.needsAction + 1 };
        case 'waiting_author':
          return { ...counts, waitingAuthor: counts.waitingAuthor + 1 };
        default:
          return { ...counts, fyi: counts.fyi + 1 };
      }
    },
    { needsAction: 0, waitingAuthor: 0, fyi: 0 },
  );
}
