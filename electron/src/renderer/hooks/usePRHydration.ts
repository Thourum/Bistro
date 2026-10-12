import { Fault, ResultAsync } from '@thourum/fault';
import { useCallback, useEffect, useRef, useState } from 'react';
import { appStore } from '../store';
import type { PR } from '../types';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('usePRHydration');
const DEFAULT_HYDRATION_ERROR = 'Unable to hydrate this pull request.';

export interface PRHydrationState {
  isHydrating: boolean;
  error: string | null;
  retry: () => void;
}

function hydrationErrorMessage(error: unknown): string {
  return error instanceof Error && error.message.trim() !== ''
    ? error.message
    : DEFAULT_HYDRATION_ERROR;
}

/**
 * Hydrates a list-only PR row on demand. PRs from very active repos are
 * synced without details (`hydrated: 0`); selecting one triggers
 * `REQUEST_PR_SYNC` so the engine fetches comments, CI, and diff for just
 * that PR. On success the list and comment revisions are bumped so open
 * pages refetch the now-complete data. No-op for already-hydrated PRs.
 */
export function usePRHydration(pr: PR | undefined): PRHydrationState {
  const { request } = useWebSocket();
  const requestRef = useRef(request);
  requestRef.current = request;
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
    void ResultAsync.fromPromise(
      requestRef.current('REQUEST_PR_SYNC', { pr_id: prId }),
      (cause) =>
        new Fault(hydrationErrorMessage(cause))
          .withTag('NETWORK_ERROR')
          .withCause(cause),
    )
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
