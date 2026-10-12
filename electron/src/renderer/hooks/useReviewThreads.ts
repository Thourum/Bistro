import { Fault } from '@thourum/fault';
import { useCallback, useEffect, useState } from 'react';
import { z } from 'zod';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { type ReviewThread, reviewThreadSchema } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';
import { useStore } from './useStore';

const log = createLogger('useReviewThreads');
const reviewThreadsResponseSchema = z.object({
  threads: z
    .array(reviewThreadSchema)
    .nullish()
    .transform((threads) => threads ?? []),
});

interface ReviewThreadsState {
  threads: ReviewThread[];
  loading: boolean;
  error: string | null;
}

/**
 * Loads the review threads for a PR via `GET_REVIEW_THREADS`.
 *
 * Mirrors the `usePatchsets` pattern: a plain WS request into local state
 * (this codebase does not route these reads through TanStack Query). An empty
 * list is a valid, non-error state — the caller renders an empty affordance.
 *
 * The hook re-fetches whenever `threadsRevision` changes, which the WebSocket
 * layer bumps on every `THREAD_RESOLVED` / `THREAD_REOPENED` push. That keeps
 * the list live after a resolve/unresolve write lands, with no full sync.
 */
export function useReviewThreads(
  prId: string | undefined,
): ReviewThreadsState & {
  refetch: () => void;
} {
  const threadsRevision = useStore(appStore, (s: UIState) => s.threadsRevision);

  const [state, setState] = useState<ReviewThreadsState>({
    threads: [],
    loading: prId !== undefined,
    error: null,
  });

  const load = useCallback(() => {
    if (prId === undefined) return;
    setState((s) => ({ ...s, loading: true, error: null }));
    void wsClient
      .request('GET_REVIEW_THREADS', { pr_id: prId })
      .mapErr((cause) =>
        new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
      )
      .andThen((data) => parseResponse(reviewThreadsResponseSchema, data))
      .match(
        (payload) => {
          setState({ threads: payload.threads, loading: false, error: null });
        },
        (message) => {
          log.warn({ err: message.message }, 'failed to load review threads');
          setState({ threads: [], loading: false, error: message.message });
        },
      );
  }, [prId]);

  // `threadsRevision` is intentionally a dependency: a push event bumps it and
  // we re-issue the read.
  // biome-ignore lint/correctness/useExhaustiveDependencies: threadsRevision is a deliberate refetch trigger, not used inside the effect body
  useEffect(() => {
    load();
  }, [load, threadsRevision]);

  return { ...state, refetch: load };
}
