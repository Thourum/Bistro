import { Interdiff } from '@bindings/services';
import { useCallback, useEffect, useRef, useState } from 'react';
import { call } from '../services/engine/call';
import type { InterdiffResult } from '../types';
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
 * Requests `Interdiff.Get` whenever `prId` or `req` changes; `req = null`
 * clears. `available: false` is a normal result; only a service or transport
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
    void call(() =>
      Interdiff.Get({
        pr_id: prId,
        path: current.path,
        from_patchset: current.fromPatchset,
        to_patchset: current.toPatchset,
      }),
    ).match(
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
