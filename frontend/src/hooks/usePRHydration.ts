import { PR as PRService } from '@bindings/services';
import { useCallback, useEffect, useRef, useState } from 'react';
import { call } from '../services/engine/call';
import { appStore } from '../store';
import type { PR } from '../types';
import { createLogger } from '../utils/logger';

const log = createLogger('usePRHydration');
const PR_SYNC_TIMEOUT_MS = 135_000;

export interface PRHydrationState {
  isHydrating: boolean;
  error: string | null;
  retry: () => void;
}

/**
 * Hydrates a list-only PR row on demand. PRs from very active repos are
 * synced without details (`hydrated: 0`); selecting one triggers
 * `REQUEST_PR_SYNC` so the engine fetches comments, CI, and diff for just
 * that PR. On success the list and comment revisions are bumped so open
 * pages refetch the now-complete data. No-op for already-hydrated PRs.
 */
export function usePRHydration(pr: PR | undefined): PRHydrationState {
  const inFlightRef = useRef<string | null>(null);
  const isMountedRef = useRef(true);
  const activePrIdRef = useRef(pr?.id);
  activePrIdRef.current = pr?.id;
  const [isHydrating, setIsHydrating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const prId = pr?.id;
  const needsHydration = pr !== undefined && pr.hydrated === 0;

  const hydrate = useCallback(() => {
    if (prId === undefined || !needsHydration) return;
    if (inFlightRef.current === prId) return;
    inFlightRef.current = prId;
    setIsHydrating(true);
    setError(null);

    log.debug({ prId }, 'hydrating list-only PR');
    void call(() => PRService.Sync({ pr_id: prId }), {
      timeoutMs: PR_SYNC_TIMEOUT_MS,
    })
      .match(
        () => {
          if (!isMountedRef.current || activePrIdRef.current !== prId) return;
          appStore.setState((state) => ({
            ...state,
            prListRevision: state.prListRevision + 1,
            commentsRevision: state.commentsRevision + 1,
          }));
          log.debug({ prId }, 'PR hydrated');
        },
        (hydrationError) => {
          if (!isMountedRef.current || activePrIdRef.current !== prId) return;
          setError(hydrationError.message);
          log.warn(
            { err: hydrationError.message, prId },
            'on-demand PR hydration failed',
          );
        },
      )
      .finally(() => {
        if (inFlightRef.current === prId) {
          inFlightRef.current = null;
        }
        if (isMountedRef.current && activePrIdRef.current === prId) {
          setIsHydrating(false);
        }
      });
  }, [needsHydration, prId]);

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  useEffect(() => {
    if (!needsHydration) {
      setError(null);
      setIsHydrating(false);
      return;
    }
    hydrate();
  }, [hydrate, needsHydration]);

  return { isHydrating, error, retry: hydrate };
}
