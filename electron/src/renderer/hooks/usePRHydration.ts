import { useEffect, useRef } from 'react';
import { appStore } from '../store';
import type { PR } from '../types';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('usePRHydration');

/**
 * Hydrates a list-only PR row on demand. PRs from very active repos are
 * synced without details (`hydrated: 0`); selecting one triggers
 * `REQUEST_PR_SYNC` so the engine fetches comments, CI, and diff for just
 * that PR. On success the list and comment revisions are bumped so open
 * pages refetch the now-complete data. No-op for already-hydrated PRs.
 */
export function usePRHydration(pr: PR | undefined): void {
  const { request } = useWebSocket();
  const requestRef = useRef(request);
  requestRef.current = request;
  const inFlightRef = useRef<string | null>(null);

  const prId = pr?.id;
  const needsHydration = pr !== undefined && pr.hydrated === 0;

  useEffect(() => {
    if (prId === undefined || !needsHydration) return;
    if (inFlightRef.current === prId) return;
    inFlightRef.current = prId;

    log.debug({ prId }, 'hydrating list-only PR');
    requestRef
      .current('REQUEST_PR_SYNC', { pr_id: prId })
      .then(() => {
        appStore.setState((s) => ({
          ...s,
          prListRevision: s.prListRevision + 1,
          commentsRevision: s.commentsRevision + 1,
        }));
        log.debug({ prId }, 'PR hydrated');
      })
      .catch((err: unknown) => {
        log.warn({ err, prId }, 'on-demand PR hydration failed');
      })
      .finally(() => {
        if (inFlightRef.current === prId) {
          inFlightRef.current = null;
        }
      });
  }, [prId, needsHydration]);
}
