import { Fault } from '@thourum/fault';
import { useCallback, useEffect, useRef, useState } from 'react';
import { wsClient } from '../services/ws/connection';
import { type InterdiffResult, interdiffResultSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('useInterdiff');

/** One file between two patchsets; `toPatchset` 0 means latest. */
export interface InterdiffRequest {
  path: string;
  fromPatchset: number;
  toPatchset: number;
}

interface InterdiffState {
  result: InterdiffResult | null;
  loading: boolean;
  error: string | null;
}

/**
 * Requests `REQUEST_INTERDIFF` whenever `prId` or `req` changes; `req = null`
 * clears. `available: false` is a normal result; only a transport or parse
 * failure populates `error`.
 */
export function useInterdiff(
  prId: string | undefined,
  req: InterdiffRequest | null,
): InterdiffState {
  const [state, setState] = useState<InterdiffState>({
    result: null,
    loading: false,
    error: null,
  });

  // `reqKey` is the real dependency; the live `req` is read through a ref.
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
    void wsClient
      .request('REQUEST_INTERDIFF', {
        pr_id: prId,
        path: current.path,
        from_patchset: current.fromPatchset,
        to_patchset: current.toPatchset,
      })
      .mapErr((cause) =>
        new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
      )
      .andThen((data) => parseResponse(interdiffResultSchema, data))
      .match(
        (result) => {
          if (!isActive) return;
          setState({ result, loading: false, error: null });
        },
        (fault) => {
          if (!isActive) return;
          log.warn({ err: fault.message }, 'failed to load interdiff');
          setState({ result: null, loading: false, error: fault.message });
        },
      );
    // A late response must not restore a disabled pane or a previous file.
    return () => {
      isActive = false;
    };
  }, [prId, reqKey]);

  useEffect(load, [load]);

  return state;
}
