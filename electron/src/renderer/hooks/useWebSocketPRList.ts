import { useQuery } from '@tanstack/react-query';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { PR } from '../types';
import { classifyPR } from '../utils/triage';
import { useStore } from './useStore';
import { useWebSocket } from './useWebSocket';

type FilterState = UIState['filterState'];

interface RequestPRListData {
  filter: 'open' | 'draft' | 'all';
  limit: number;
  offset: number;
}

interface PRListReadyData {
  prs: PR[];
  total: number;
  has_more: boolean;
}

/**
 * Fetches PR list from sync engine via WebSocket
 * Replaces SQLite-based queries with live sync engine data
 *
 * 'needs_action' and 'touches_you' are client-side lanes: the backend serves
 * open PRs and the list is narrowed locally.
 */
export function useWebSocketPRList(filterState: FilterState = 'open') {
  const { request } = useWebSocket();
  const prListRevision = useStore(appStore, (s) => s.prListRevision);
  const username = useStore(
    appStore,
    (s) => (s.connectedProviders ?? [])[0]?.username ?? '',
  );

  return useQuery({
    queryKey: ['prs-websocket', filterState, username, prListRevision],
    queryFn: async (): Promise<PR[]> => {
      const backendFilter: RequestPRListData['filter'] =
        filterState === 'needs_action' || filterState === 'touches_you'
          ? 'open'
          : filterState;
      try {
        const data = await request('REQUEST_PR_LIST', {
          filter: backendFilter,
          limit: 100,
          offset: 0,
        } as RequestPRListData);

        const payload = data as PRListReadyData;
        if (filterState === 'needs_action') {
          return payload.prs.filter(
            (pr) => classifyPR(pr, username) === 'needs_action',
          );
        }
        if (filterState === 'touches_you') {
          return payload.prs.filter((pr) => pr.touches_you);
        }
        return payload.prs;
      } catch (err) {
        const message =
          err instanceof Error ? err.message : 'Failed to fetch PR list';
        throw new Error(message);
      }
    },
    staleTime: 30 * 1000, // 30s
    gcTime: 5 * 60 * 1000, // 5m
    retry: 1,
  });
}
