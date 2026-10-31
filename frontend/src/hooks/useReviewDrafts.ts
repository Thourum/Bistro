import { useEffect } from 'react';
import { loadDrafts } from '../services/reviewDrafts';
import { appStore } from '../store';
import { createLogger } from '../utils/logger';

const log = createLogger('useReviewDrafts');

/** Keeps appStore.reviewDrafts loaded for the PR the page shows. */
export function useReviewDrafts(prId: string | undefined): void {
  useEffect(() => {
    if (prId === undefined) return;
    appStore.setState((s) => ({
      ...s,
      reviewDraftsPrId: prId,
      reviewDrafts: [],
    }));
    void loadDrafts(prId).mapErr((fault) =>
      log.warn({ err: fault.message }, 'failed to load review drafts'),
    );
    return () => {
      appStore.setState((s) =>
        s.reviewDraftsPrId === prId
          ? { ...s, reviewDraftsPrId: null, reviewDrafts: [] }
          : s,
      );
    };
  }, [prId]);
}
