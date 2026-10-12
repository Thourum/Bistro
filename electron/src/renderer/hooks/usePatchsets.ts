import { Fault, fromPromise } from '@thourum/fault';
import { useCallback, useEffect, useRef, useState } from 'react';
import { z } from 'zod';
import { type Patchset, patchsetSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('usePatchsets');
const patchsetsResponseSchema = z.object({
  patchsets: z
    .array(patchsetSchema)
    .nullish()
    .transform((patchsets) => patchsets ?? []),
});

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
    void fromPromise(
      requestRef.current('GET_PATCHSETS', { pr_id: prId }),
      (cause) =>
        new Fault(cause instanceof Error ? cause : 'Unknown error')
          .withTag('NETWORK_ERROR')
          .withCause(cause),
    )
      .andThen((data) => parseResponse(patchsetsResponseSchema, data))
      .match(
        (payload) => {
          setState({
            patchsets: payload.patchsets,
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
