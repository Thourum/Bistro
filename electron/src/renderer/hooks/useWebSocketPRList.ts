import { useQuery } from '@tanstack/react-query';
import { useEffect } from 'react';
import { appStore } from '../store';
import type { PR } from '../types';
import { useStore } from './useStore';
import { useWebSocket } from './useWebSocket';

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
 */
export function useWebSocketPRList(
  filterState: 'open' | 'draft' | 'all' = 'open',
) {
  const { request } = useWebSocket();
  const prListRevision = useStore(appStore, (s) => s.prListRevision);

  return useQuery({
    queryKey: ['prs-websocket', filterState, prListRevision],
    queryFn: async (): Promise<PR[]> => {
      try {
        const data = await request('REQUEST_PR_LIST', {
          filter: filterState,
          limit: 100,
          offset: 0,
        } as RequestPRListData);

        const payload = data as PRListReadyData;
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

/**
 * Hook to refetch PR list when sync completes
 * Subscribes to SYNC_COMPLETE events and invalidates query cache
 */
export function useWebSocketSyncListener(
  onSyncComplete?: (stats: {
    prCount: number;
    newPRs: number;
    updatedPRs: number;
  }) => void,
): void {
  const syncStats = useStore(appStore, (s) => s.syncStats);

  useEffect(() => {
    if (syncStats !== null) {
      onSyncComplete?.(syncStats);
    }
  }, [syncStats, onSyncComplete]);
}
