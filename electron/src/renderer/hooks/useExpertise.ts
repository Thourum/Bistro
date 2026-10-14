import { Fault } from '@thourum/fault';
import { useCallback, useEffect, useRef, useState } from 'react';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import { type ExpertiseDataPayload, expertiseDataSchema } from '../types';
import { parseResponse } from '../utils/error';
import {
  expertisePayloadForPR,
  isExpertisePayloadForPR,
} from '../utils/expertise';
import { createLogger } from '../utils/logger';
import { useStore } from './useStore';

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
  const prListRevision = useStore(appStore, (s) => s.prListRevision);
  const latestRequestIdRef = useRef(0);
  const lastReadyRevisionRef = useRef(prListRevision);

  const [state, setState] = useState<ExpertiseState>({
    expertise: null,
    loading: prId !== undefined,
    error: null,
  });

  const load = useCallback(() => {
    if (prId === undefined) {
      latestRequestIdRef.current += 1;
      setState({ expertise: null, loading: false, error: null });
      return;
    }
    const requestId = latestRequestIdRef.current + 1;
    latestRequestIdRef.current = requestId;
    setState({ expertise: null, loading: true, error: null });
    void wsClient
      .request('GET_EXPERTISE', { pr_id: prId })
      .mapErr((cause) =>
        new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
      )
      .andThen((data) => parseResponse(expertiseDataSchema, data))
      .match(
        (data) => {
          if (latestRequestIdRef.current !== requestId) return;
          const payload = expertisePayloadForPR(data, prId);
          setState({ expertise: payload, loading: false, error: null });
        },
        (message) => {
          if (latestRequestIdRef.current !== requestId) return;
          log.warn({ err: message.message }, 'failed to load expertise');
          setState({ expertise: null, loading: false, error: message.message });
        },
      );
  }, [prId]);

  useEffect(() => {
    load();
  }, [load]);

  const lastPrIdRef = useRef(prId);
  if (lastPrIdRef.current !== prId) {
    lastPrIdRef.current = prId;
    lastReadyRevisionRef.current = appStore.state.prListRevision;
    if (prId === undefined) {
      latestRequestIdRef.current += 1;
    }
  }

  useEffect(() => {
    if (prId === undefined || prListRevision === lastReadyRevisionRef.current) {
      return;
    }
    if (
      isExpertisePayloadForPR(state.expertise, prId) &&
      state.expertise.status !== 'unavailable'
    ) {
      lastReadyRevisionRef.current = prListRevision;
      return;
    }
    lastReadyRevisionRef.current = prListRevision;
    load();
  }, [load, prId, prListRevision, state.expertise]);

  return { ...state, refetch: load };
}
