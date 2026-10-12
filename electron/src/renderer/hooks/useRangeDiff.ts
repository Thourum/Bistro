import { Fault, fromPromise } from '@thourum/fault';
import { useCallback, useEffect, useRef, useState } from 'react';
import { type RangeDiffResult, rangeDiffSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('useRangeDiff');

/**
 * What to diff. Either an explicit patchset pair (from the timeline selector)
 * or the "since last review" range, where the engine substitutes the
 * reviewer's last-reviewed SHA as the base.
 */
export type RangeDiffRequest =
  | { kind: 'pair'; fromPatchset: number; toPatchset: number }
  | { kind: 'sinceLastReview' };

interface RangeDiffState {
  result: RangeDiffResult | null;
  loading: boolean;
  error: string | null;
}

/**
 * Requests a range diff via `REQUEST_RANGE_DIFF` whenever `prId` or `req`
 * changes. Pass `req = null` to clear / skip.
 *
 * A `degraded` result is NOT an error — it is delivered as a normal result
 * and the view layer decides how to present it. Only a transport-level
 * failure populates `error`.
 */
export function useRangeDiff(
  prId: string | undefined,
  req: RangeDiffRequest | null,
): RangeDiffState {
  const { request } = useWebSocket();
  const requestRef = useRef(request);
  requestRef.current = request;

  const [state, setState] = useState<RangeDiffState>({
    result: null,
    loading: false,
    error: null,
  });

  // `reqKey` is the serialized request; it is the real dependency. The live
  // `req` object is read through a ref so the callback need not list each of
  // its fields (which all change together with `reqKey`).
  const reqKey = req === null ? null : JSON.stringify(req);
  const reqRef = useRef(req);
  reqRef.current = req;

  const load = useCallback(() => {
    // `reqKey` is read here so it is a genuine dependency: a changed request
    // (different patchset pair, or toggling since-last-review) refetches. The
    // live `req` is pulled from a ref to avoid listing each of its fields.
    void reqKey;
    const current = reqRef.current;
    if (prId === undefined || current === null) {
      setState({ result: null, loading: false, error: null });
      return;
    }
    setState((s) => ({ ...s, loading: true, error: null }));

    const payload =
      current.kind === 'sinceLastReview'
        ? { pr_id: prId, since_last_review: true }
        : {
            pr_id: prId,
            from_patchset: current.fromPatchset,
            to_patchset: current.toPatchset,
            since_last_review: false,
          };

    void fromPromise(
      requestRef.current('REQUEST_RANGE_DIFF', payload),
      (cause) =>
        new Fault(cause instanceof Error ? cause : 'Unknown error')
          .withTag('NETWORK_ERROR')
          .withCause(cause),
    )
      .andThen((data) => parseResponse(rangeDiffSchema, data))
      .match(
        (result) => {
          setState({ result, loading: false, error: null });
        },
        (message) => {
          log.warn({ err: message.message }, 'failed to load range diff');
          setState({ result: null, loading: false, error: message.message });
        },
      );
  }, [prId, reqKey]);

  useEffect(() => {
    load();
  }, [load]);

  return state;
}
