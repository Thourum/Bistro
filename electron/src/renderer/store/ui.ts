import { Store } from '@tanstack/store';
import type { PendingAction, ActionResult, Toast } from '../types';

export interface SyncStats {
  prCount: number;
  newPRs: number;
  updatedPRs: number;
}

export interface UIState {
  selectedPRIndex: number;
  selectedFileIndex: number;
  searchQuery: string;
  mode: 'list' | 'detail' | 'search' | 'help' | 'command' | 'filter' | 'settings';
  isAuthenticated: boolean;
  darkMode: boolean;
  showFileTree: boolean;
  autoSync: boolean;
  syncInterval: number;
  notifications: boolean;
  paneLayout: 'list' | 'detail' | 'split';
  sortBy: 'updated' | 'created';
  filterState: 'open' | 'draft' | 'all';
  confirmingAction: string | null;
  filterMenuOpen: boolean;
  pendingAction: PendingAction | null;
  actionResult: ActionResult | null;
  // Legacy sync state (kept for compatibility)
  isSyncing: boolean;
  syncProgress: number;
  lastSyncedAt: Date | null;
  // WebSocket sync state
  syncStatus: 'disconnected' | 'connecting' | 'connected' | 'ready' | 'error' | 'rate_limited';
  syncError: string | null;
  lastSyncAt: string | null;
  syncStats: SyncStats | null;
  toasts: Toast[];
  isOffline: boolean;
}

const SETTINGS_STORAGE_KEY = 'bistro-settings';

function loadPersistedSettings(): Pick<UIState, 'darkMode' | 'showFileTree' | 'autoSync' | 'syncInterval' | 'notifications'> {
  try {
    const raw = localStorage.getItem(SETTINGS_STORAGE_KEY);
    if (raw !== null) {
      return JSON.parse(raw) as Pick<UIState, 'darkMode' | 'showFileTree' | 'autoSync' | 'syncInterval' | 'notifications'>;
    }
  } catch {
    // ignore parse errors
  }
  return { darkMode: true, showFileTree: false, autoSync: true, syncInterval: 60, notifications: true };
}

export function persistSettings(state: UIState): void {
  try {
    const settings: Pick<UIState, 'darkMode' | 'showFileTree' | 'autoSync' | 'syncInterval' | 'notifications'> = {
      darkMode: state.darkMode,
      showFileTree: state.showFileTree,
      autoSync: state.autoSync,
      syncInterval: state.syncInterval,
      notifications: state.notifications,
    };
    localStorage.setItem(SETTINGS_STORAGE_KEY, JSON.stringify(settings));
  } catch {
    // ignore storage errors
  }
}

const initialUIState: UIState = {
  selectedPRIndex: 0,
  selectedFileIndex: 0,
  searchQuery: '',
  mode: 'list',
  paneLayout: 'split',
  sortBy: 'updated',
  filterState: 'open',
  confirmingAction: null,
  filterMenuOpen: false,
  pendingAction: null,
  actionResult: null,
  isAuthenticated: false,
  isSyncing: false,
  syncProgress: 0,
  lastSyncedAt: null,
  syncStatus: 'disconnected',
  syncError: null,
  lastSyncAt: null,
  syncStats: null,
  toasts: [],
  isOffline: false,
  ...loadPersistedSettings(),
};

export function createUIStore(): Store<UIState> {
  return new Store<UIState>(initialUIState);
}
