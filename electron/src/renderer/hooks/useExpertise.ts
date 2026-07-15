import { useCallback, useEffect, useRef, useState } from 'react';
import type { ExpertiseDataPayload } from '../types';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('useExpertise');

interface ExpertiseState {
  expertise: ExpertiseDataPayload | null;
  loading: boolean;
  error: string | null;
}

/**
 * Loads per-file local expertise explanations for a PR via `GET_EXPERTISE`.
 *
 * Unavailable expertise is a valid payload (`status: 'unavailable'`) so callers
 * can stay quiet unless the repo itself reports an expertise error.
 */
export function useExpertise(prId: string | undefined): ExpertiseState & {
  refetch: () => void;
} {
  const { request } = useWebSocket();
  const requestRef = useRef(request);
  requestRef.current = request;

  const [state, setState] = useState<ExpertiseState>({
    expertise: null,
    loading: prId !== undefined,
    error: null,
  });

  const load = useCallback(() => {
    if (prId === undefined) return;
    setState((s) => ({ ...s, loading: true, error: null }));
    requestRef
      .current('GET_EXPERTISE', { pr_id: prId })
      .then((data) => {
        const payload = data as ExpertiseDataPayload;
        setState({
          expertise: {
            ...payload,
            pr_id: payload.pr_id ?? prId,
            files: payload.files ?? [],
          },
          loading: false,
          error: null,
        });
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        log.warn({ err: msg }, 'failed to load expertise');
        setState({ expertise: null, loading: false, error: msg });
      });
  }, [prId]);

  useEffect(() => {
    load();
  }, [load]);

  return { ...state, refetch: load };
}
