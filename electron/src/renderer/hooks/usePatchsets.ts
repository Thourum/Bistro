import { useCallback, useEffect, useRef, useState } from 'react';
import type { Patchset } from '../types';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('usePatchsets');

interface PatchsetsState {
  patchsets: Patchset[];
  loading: boolean;
  error: string | null;
}

/**
 * Loads the observed patchset history for a PR via `GET_PATCHSETS`.
 *
 * Returns the patchsets ordered as the engine sends them (ascending by
 * number). An empty list is a valid, non-error state — the caller renders a
 * "no snapshots yet" affordance. `refetch` re-issues the request.
 */
export function usePatchsets(prId: string | undefined): PatchsetsState & {
  refetch: () => void;
} {
  const { request } = useWebSocket();
  // Hold the latest `request` in a ref so `load` stays stable even if the
  // hook's identity changes between renders (avoids resolve→setState→render
  // loops).
  const requestRef = useRef(request);
  requestRef.current = request;

  const [state, setState] = useState<PatchsetsState>({
    patchsets: [],
    loading: prId !== undefined,
    error: null,
  });

  const load = useCallback(() => {
    if (prId === undefined) return;
    setState((s) => ({ ...s, loading: true, error: null }));
    requestRef
      .current('GET_PATCHSETS', { pr_id: prId })
      .then((data) => {
        const payload = data as { patchsets?: Patchset[] };
        setState({
          patchsets: payload.patchsets ?? [],
          loading: false,
          error: null,
        });
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        log.warn({ err: msg }, 'failed to load patchsets');
        setState({ patchsets: [], loading: false, error: msg });
      });
  }, [prId]);

  useEffect(() => {
    load();
  }, [load]);

  return { ...state, refetch: load };
}
