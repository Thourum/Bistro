import { Store } from '@tanstack/store';
import type { ActionResult, PendingAction, RepoInfo, Toast } from '../types';

export interface SyncStats {
  prCount: number;
  newPRs: number;
  updatedPRs: number;
}

/**
 * Transient UI state (not persisted).
 *
 * All user preferences live in `settingsStore` (store/settings.ts) and are
 * round-tripped through the main process into `userData/settings.json`.
 */
export interface UIState {
  selectedPRIndex: number;
  selectedFileIndex: number;
  searchQuery: string;
  mode:
    | 'login'
    | 'onboarding'
    | 'list'
    | 'detail'
    | 'diff'
    | 'search'
    | 'help'
    | 'command'
    | 'filter'
    | 'repo-picker'
    | 'settings';
  /** Mode the user was in when opening the help overlay — scopes its content. */
  helpSourceMode: 'list' | 'detail';
  isAuthenticated: boolean;
  showFileTree: boolean;
  paneLayout: 'list' | 'detail' | 'split';
  sortBy: 'updated' | 'created';
  filterState: 'open' | 'draft' | 'all' | 'needs_action';
  confirmingAction: string | null;
  filterMenuOpen: boolean;
  pendingAction: PendingAction | null;
  actionResult: ActionResult | null;
  // WebSocket sync state
  syncStatus:
    | 'disconnected'
    | 'connecting'
    | 'connected'
    | 'ready'
    | 'syncing'
    | 'error'
    | 'rate_limited';
  syncError: string | null;
  lastSyncAt: string | null;
  prListRevision: number;
  syncStats: SyncStats | null;
  toasts: Toast[];
  isOffline: boolean;
  // OAuth auth state
  authLoading: boolean;
  authError: string | null;
  authChecked: boolean;
  connectedProviders: Array<{ provider: string; username: string }>;
  // Repo discovery
  repos: RepoInfo[];
  reposLoading: boolean;
  repoFilter: string | null;
}

export const initialUIState: UIState = {
  selectedPRIndex: 0,
  selectedFileIndex: 0,
  searchQuery: '',
  mode: 'list',
  helpSourceMode: 'list',
  paneLayout: 'split',
  sortBy: 'updated',
  filterState: 'open',
  confirmingAction: null,
  filterMenuOpen: false,
  pendingAction: null,
  actionResult: null,
  isAuthenticated: false,
  showFileTree: false,
  syncStatus: 'disconnected',
  syncError: null,
  lastSyncAt: null,
  prListRevision: 0,
  syncStats: null,
  toasts: [],
  isOffline: false,
  authLoading: false,
  authError: null,
  authChecked: false,
  connectedProviders: [],
  repos: [],
  reposLoading: false,
  repoFilter: null,
};

export function createUIStore(): Store<UIState> {
  return new Store<UIState>(initialUIState);
}
