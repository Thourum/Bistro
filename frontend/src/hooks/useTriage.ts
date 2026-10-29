import { Fault } from '@thourum/fault';
import { useCallback, useEffect, useRef, useState } from 'react';
import { wsClient } from '../services/ws/connection';
import { type TriageResult, triageResultSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('useTriage');

/** Triage against the reviewer's last review, or an explicit patchset pair. */
export type TriageRequest =
  | { kind: 'sinceLastReview' }
  | { kind: 'pair'; fromPatchset: number; toPatchset: number };

interface TriageState {
  result: TriageResult | null;
  loading: boolean;
  error: string | null;
}

/**
 * Requests `REQUEST_TRIAGE` whenever `prId` or `req` changes; `req = null`
 * clears. A non-`ok` triage state is a normal result; only a transport or
 * parse failure populates `error`.
 */
export function useTriage(
  prId: string | undefined,
  req: TriageRequest | null,
): TriageState {
  const [state, setState] = useState<TriageState>({
    result: null,
    loading: false,
    error: null,
  });

  // `reqKey` is the real dependency; the live `req` is read through a ref so
  // the callback need not list each of its fields.
  const reqKey = req === null ? null : JSON.stringify(req);
  const reqRef = useRef(req);
  reqRef.current = req;

  const load = useCallback(() => {
    void reqKey;
    const current = reqRef.current;
    if (prId === undefined || current === null) {
      setState({ result: null, loading: false, error: null });
      return;
    }
    setState({ result: null, loading: true, error: null });
    let isActive = true;
    const payload =
      current.kind === 'sinceLastReview'
        ? { pr_id: prId, from_patchset: 0, to_patchset: 0 }
        : {
            pr_id: prId,
            from_patchset: current.fromPatchset,
            to_patchset: current.toPatchset,
          };

    void wsClient
      .request('REQUEST_TRIAGE', payload)
      .mapErr((cause) =>
        new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
      )
      .andThen((data) => parseResponse(triageResultSchema, data))
      .match(
        (result) => {
          if (!isActive) return;
          setState({ result, loading: false, error: null });
        },
        (fault) => {
          if (!isActive) return;
          log.warn({ err: fault.message }, 'failed to load triage');
          setState({ result: null, loading: false, error: fault.message });
        },
      );
    // A late response must not restore a disabled filter or an older pair.
    return () => {
      isActive = false;
    };
  }, [prId, reqKey]);

  useEffect(load, [load]);

  return state;
}
