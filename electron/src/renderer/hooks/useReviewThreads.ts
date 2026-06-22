import { useCallback, useEffect, useRef, useState } from 'react';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { ReviewThread } from '../types';
import { createLogger } from '../utils/logger';
import { useStore } from './useStore';
import { useWebSocket } from './useWebSocket';

const log = createLogger('useReviewThreads');

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
  const { request } = useWebSocket();
  const requestRef = useRef(request);
  requestRef.current = request;

  const threadsRevision = useStore(appStore, (s: UIState) => s.threadsRevision);

  const [state, setState] = useState<ReviewThreadsState>({
    threads: [],
    loading: prId !== undefined,
    error: null,
  });

  const load = useCallback(() => {
    if (prId === undefined) return;
    setState((s) => ({ ...s, loading: true, error: null }));
    requestRef
      .current('GET_REVIEW_THREADS', { pr_id: prId })
      .then((data) => {
        const payload = data as { threads?: ReviewThread[] };
        setState({
          threads: payload.threads ?? [],
          loading: false,
          error: null,
        });
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        log.warn({ err: msg }, 'failed to load review threads');
        setState({ threads: [], loading: false, error: msg });
      });
  }, [prId]);

  // `threadsRevision` is intentionally a dependency: a push event bumps it and
  // we re-issue the read.
  // biome-ignore lint/correctness/useExhaustiveDependencies: threadsRevision is a deliberate refetch trigger, not used inside the effect body
  useEffect(() => {
    load();
  }, [load, threadsRevision]);

  return { ...state, refetch: load };
}
