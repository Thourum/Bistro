import { Threads } from '@bindings/services';
import { useCallback, useEffect, useState } from 'react';
import { call } from '../services/engine/call';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { ReviewThread } from '../types';
import { createLogger } from '../utils/logger';
import { useStore } from './useStore';

const log = createLogger('useReviewThreads');

interface ReviewThreadsState {
  threads: ReviewThread[];
  loading: boolean;
  error: string | null;
}

/**
 * Loads the review threads for a PR via `Threads.List`.
 *
 * Mirrors the `usePatchsets` pattern: a typed engine call into local state
 * (this codebase does not route these reads through TanStack Query). An empty
 * list is a valid, non-error state — the caller renders an empty affordance.
 *
 * The hook re-fetches whenever `threadsRevision` changes, which engine
 * events bump on each thread state change. That keeps
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
    void call(() => Threads.List({ pr_id: prId })).match(
      (payload) => {
        setState({
          threads: payload.threads ?? [],
          loading: false,
          error: null,
        });
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
