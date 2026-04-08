import { useEffect } from 'react';
import { appStore } from '../store';
import type { SyncEvent } from '../../main/websocket-client';
import { createLogger } from '../utils/logger';

const log = createLogger('SyncEvents');

function handleSyncEvent(event: SyncEvent): void {
  switch (event.type) {
    case 'sync_started':
      try {
        appStore.setState((state) => ({
          ...state,
          isSyncing: true,
          syncProgress: 0,
        }));
      } catch {
        // Store not available (test environment)
      }
      break;

    case 'progress': {
      const current = event.data?.current as number | undefined;
      const total = event.data?.total as number | undefined;
      const progress =
        current !== undefined && total !== undefined && total > 0
          ? Math.round((current / total) * 100)
          : 0;
      try {
        appStore.setState((state) => ({
          ...state,
          syncProgress: progress,
        }));
      } catch {
        // Store not available (test environment)
      }
      break;
    }

    case 'sync_complete':
      try {
        appStore.setState((state) => ({
          ...state,
          isSyncing: false,
          syncProgress: 100,
          lastSyncedAt: new Date(event.timestamp),
        }));
      } catch {
        // Store not available (test environment)
      }
      break;

    case 'action_complete':
      log.info({ data: event.data }, 'action complete');
      break;

    case 'action_failed':
      log.error({ data: event.data }, 'action failed');
      break;

    default:
      log.warn({ type: (event as SyncEvent).type }, 'unknown event type');
  }
}

/**
 * Subscribes to sync events from the Go backend (via Electron main process).
 * Updates the app store with real-time sync state.
 * Call this once at the top of PRListPage to activate event listening.
 */
export function useSyncEvents(): void {
  useEffect(() => {
    // Only available in Electron renderer with preload script
    if (typeof window === 'undefined' || !('electron' in window)) {
      return;
    }

    // Declare interest in sync events
    window.electron.send('sync:subscribe');

    // Subscribe to incoming sync events
    const unsubscribe = window.electron.on('sync:event', handleSyncEvent);

    return () => {
      unsubscribe();
    };
  }, []);
}
