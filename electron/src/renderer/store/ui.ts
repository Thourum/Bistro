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
  isAuthenticated: boolean;
  showFileTree: boolean;
  paneLayout: 'list' | 'detail' | 'split';
  sortBy: 'updated' | 'created';
  filterState: 'open' | 'draft' | 'all' | 'needs_action';
  confirmingAction: string | null;
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
  /**
   * Stable id list of the currently visible PRs — kept in sync by PRListPage.
   * Used by keybinding actions (list.open, list.bottom) to navigate by id
   * without needing a hook context.
   */
  visiblePRIds: string[];
  /**
   * Canonical file keys for the diff currently loaded in DiffPage.
   * Set by DiffPage via onFilesResolved when PierreDiffView parses the diff.
   * Used by n/p keybindings (detail.nextFile / detail.prevFile) in diff mode
   * to navigate the ?file search param without needing a hook context.
   */
  diffFileKeys: string[];
  /**
   * Patchset pair selected in PatchsetTimeline for range diff.
   * [fromPatchsetNumber, toPatchsetNumber] — null when no pair is selected.
   */
  selectedPatchsetPair: [number, number] | null;
}

export const initialUIState: UIState = {
  selectedPRIndex: 0,
  selectedFileIndex: 0,
  searchQuery: '',
  paneLayout: 'split',
  sortBy: 'updated',
  filterState: 'open',
  confirmingAction: null,
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
  visiblePRIds: [],
  diffFileKeys: [],
  selectedPatchsetPair: null,
};

export function createUIStore(): Store<UIState> {
  return new Store<UIState>(initialUIState);
}
