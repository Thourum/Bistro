import { Patchsets } from '@bindings/services';
import { useCallback, useEffect, useState } from 'react';
import { call } from '../services/engine/call';
import type { Patchset } from '../types';
import { createLogger } from '../utils/logger';

const log = createLogger('usePatchsets');

interface PatchsetsState {
  patchsets: Patchset[];
  loading: boolean;
  error: string | null;
}

/**
 * Loads the observed patchset history for a PR via `Patchsets.List`.
 *
 * Returns the patchsets ordered as the engine sends them (ascending by
 * number). An empty list is a valid, non-error state — the caller renders a
 * "no snapshots yet" affordance. `refetch` re-issues the request.
 */
export function usePatchsets(prId: string | undefined): PatchsetsState & {
  refetch: () => void;
} {
  const [state, setState] = useState<PatchsetsState>({
    patchsets: [],
    loading: prId !== undefined,
    error: null,
  });

  const load = useCallback(() => {
    if (prId === undefined) return;
    setState((s) => ({ ...s, loading: true, error: null }));
    void call(() => Patchsets.List({ pr_id: prId })).match(
      (payload) => {
        setState({
          patchsets: payload.patchsets ?? [],
          loading: false,
          error: null,
        });
      },
      (message) => {
        log.warn({ err: message.message }, 'failed to load patchsets');
        setState({ patchsets: [], loading: false, error: message.message });
      },
    );
  }, [prId]);

  useEffect(() => {
    load();
  }, [load]);

  return { ...state, refetch: load };
}
