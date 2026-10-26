import { Store } from '@tanstack/store';
import type { EngineStatus } from '../../types/engine';
import type { FolderScanState } from '../services/onboardingSync';
import type {
  ActionResult,
  ExpertiseNudgePayload,
  ExpertiseRepoStatus,
  ExpertiseSettingsPayload,
  PendingAction,
  RepoInfo,
  ReviewDraft,
  Toast,
} from '../types';
import {
  type FirstRunRailState,
  INITIAL_FIRST_RUN_RAIL,
} from '../utils/firstRunRail';
import type { LaneSection } from '../utils/lanes';
import type { LineCommentState } from '../utils/lineCommentCursor';
import type { OnboardingSelection } from '../utils/onboardingRepos';
import type { SandboxState } from '../utils/onboardingSandbox';

export interface OnboardingSyncState {
  readonly selection: OnboardingSelection;
  readonly initialReposPending: boolean;
  readonly repoError: string | null;
  readonly queuePending: boolean;
  readonly queueError: string | null;
  readonly folder: FolderScanState;
}

interface SyncStats {
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
export type ConfirmingAction =
  | 'command'
  | 'comment'
  | 'draft'
  | 'mark-viewed'
  | 'mute'
  | 'onboarding_continue'
  | 'open-in-editor'
  | 'reply'
  | 'request_changes'
  | 'star'
  | 'toggle-diff-style';

export interface UIState {
  selectedPRIndex: number;
  selectedFileIndex: number;
  /** PR whose file list `selectedFileIndex` points into; a different PR resets it. */
  selectedFilePrId: string | null;
  searchQuery: string;
  isAuthenticated: boolean;
  showFileTree: boolean;
  paneLayout: 'list' | 'detail' | 'split';
  sortBy: 'updated' | 'created';
  filterState: 'open' | 'draft' | 'all' | 'needs_action' | 'touches_you';
  /** Onboarding style sandbox; null when the step isn't showing. */
  onboardingSandbox: SandboxState | null;
  /** Sandbox's viewDiff or sinceLastReview task was done this session; never persisted. */
  onboardingPracticedDiff: boolean;
  /** Live first-run rail steps done this session; only dismissal persists. */
  firstRunRail: FirstRunRailState;
  /** A rail dismissal write is in flight; blocks a duplicate write. */
  firstRunDismissPending: boolean;
  onboardingSync: OnboardingSyncState | null;
  onboardingStep: 'sync' | 'practice' | null;
  onboardingIntent: 'chooseFolder' | 'stopScan' | 'focusRepoFilter' | null;
  confirmingAction: ConfirmingAction | null;
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
  /**
   * Bumped whenever comments may have changed remotely or via a local action.
   * Detail/diff pages depend on this to re-request comments without requiring
   * navigation or a full PR-list sync.
   */
  commentsRevision: number;
  syncStats: SyncStats | null;
  toasts: Toast[];
  isOffline: boolean;
  // OAuth auth state
  authLoading: boolean;
  authStartRequestId: string | null;
  authError: string | null;
  authUrl: string | null;
  authStartedAt: number | null;
  authTimedOut: boolean;
  authInterrupted: boolean;
  /** Transport state; syncStatus 'error' can also mean an open socket. */
  isWsOpen: boolean;
  authExchange: 'proxy' | 'local' | null;
  engineStatus: EngineStatus | null;
  appVersion: string | null;
  authChecked: boolean;
  connectedProviders: Array<{ provider: string; username: string }>;
  /** Viewer login returned by the PR-list payload. */
  viewerLogin: string;
  // Repo discovery
  repos: RepoInfo[];
  reposLoading: boolean;
  repoFilter: string | null;
  /**
   * Stable id list and browser URLs for the currently visible PRs — kept in
   * sync by PRListPage. Used by keybinding actions (list.open,
   * list.openBrowser, list.bottom) without needing a hook context.
   */
  visiblePRIds: string[];
  /** Lane positions for the flat inbox list; empty in grouped view. */
  laneSectionsSnapshot: LaneSection[];
  visiblePRBrowserUrls: string[];
  /**
   * Repo key (`owner/repo`) for each visible row — parallel to visiblePRIds.
   * A collapsed repo group occupies a single header row whose visiblePRIds
   * entry is '' (no PR to open). Used by the Space keybinding to toggle the
   * group under the cursor.
   */
  visiblePRRepoKeys: string[];
  /** Repo keys (`owner/repo`) whose PR-list group is currently collapsed. */
  collapsedRepoKeys: string[];
  /**
   * Canonical file keys for the diff currently loaded in DiffPage.
   * Set by DiffPage via onFilesResolved when the diff view parses the diff.
   * Used by the diff.nextFile / diff.prevFile (Shift+J / Shift+K) keybindings
   * to navigate the ?file search param without needing a hook context.
   */
  diffFileKeys: string[];
  /**
   * Patchset pair selected in the timeline, as 1-based patchset numbers
   * `[from, to]`. Filters the diff to changed/new files between patchsets.
   * Null = full PR diff (unless since-last-review is active).
   */
  selectedPatchsetPair: [number, number] | null;
  /** Diff filter mode (`s`), retained across review actions and reset on exit. */
  sinceLastReviewActive: boolean;
  /** `i` in the diff view: show the current file's interdiff instead of the PR diff. */
  interdiffActive: boolean;
  /**
   * Bumped every time a `THREAD_RESOLVED` / `THREAD_REOPENED` push event
   * arrives. The threads view depends on this so a remote state change (or our
   * own write landing) re-fetches the thread list without a full sync.
   */
  threadsRevision: number;
  /**
   * Ordered review-thread ids currently rendered as inline diff annotations.
   * Set by PierreDiffView; consumed by centralized diff hotkeys.
   */
  inlineCommentThreadIds: string[];
  /** Current user's pending line-comment drafts for `reviewDraftsPrId`. */
  reviewDrafts: ReviewDraft[];
  /** PR whose drafts `reviewDrafts` holds; set by useReviewDrafts. */
  reviewDraftsPrId: string | null;
  /** Line-pick overlay state (diff `c`); null when comment mode is closed. */
  lineComment: LineCommentState | null;
  /** Currently selected inline diff-comment thread, if any. */
  activeInlineCommentThreadId: string | null;
  /** Inline diff-comment thread currently showing the reply composer, if any. */
  replyingInlineCommentThreadId: string | null;
  /**
   * Diff view: hide hunks whose only change is whitespace (`w`). Route-local
   * and deliberately not persisted — it answers "is this churn?" for one PR.
   */
  hideWhitespaceHunks: boolean;
  /** True while the keyboard settings page records a new binding; Esc then cancels recording instead of leaving settings. */
  isRecordingKeybinding: boolean;
  /**
   * Canonical file keys whose default collapse state is inverted by `z`.
   * Generated files start collapsed; everything else starts expanded.
   */
  diffCollapseOverrides: string[];
  /**
   * Comment classification filter (Phase 3.5 `f` keystrokes): minimum
   * severity (`f c/w/s/n`) and human-only (`f h`). Cleared by `f f`.
   */
  commentMinSeverity: 'critical' | 'warning' | 'suggestion' | 'nit' | null;
  commentHumanOnly: boolean;
  /**
   * Session undo stack of dismiss batches (Phase 3.5 `u`). Does not survive
   * restart by design — the SQLite dismissal rows do.
   */
  dismissBatches: Array<{
    batch_id: string;
    dismissed_ids: string[];
    resolved_thread_ids: string[];
  }>;
  /** BYOK session usage (Phase 3.8), fed by pushed BYOK_USAGE events. */
  byokSessionTokens: number;
  byokEstCost: number;
  /** Set by BYOK_AUTH_FAILED; cleared on a successful SET_BYOK_KEY ack. */
  byokAuthFailed: boolean;
  /** Per-repo expertise indexing state, keyed by repo_id. */
  expertiseRepoStatuses: Record<string, ExpertiseRepoStatus>;
  /** Pending expertise nudges, keyed by repo_id. */
  expertiseNudges: Record<string, ExpertiseNudgePayload>;
  /** Latest server-backed expertise settings payload. */
  expertiseSettings: ExpertiseSettingsPayload | null;
}

export const initialUIState: UIState = {
  selectedPRIndex: 0,
  selectedFileIndex: 0,
  selectedFilePrId: null,
  searchQuery: '',
  paneLayout: 'split',
  sortBy: 'updated',
  filterState: 'open',
  onboardingSandbox: null,
  onboardingPracticedDiff: false,
  firstRunRail: INITIAL_FIRST_RUN_RAIL,
  firstRunDismissPending: false,
  onboardingSync: null,
  onboardingStep: null,
  onboardingIntent: null,
  confirmingAction: null,
  pendingAction: null,
  actionResult: null,
  isAuthenticated: false,
  showFileTree: false,
  syncStatus: 'disconnected',
  syncError: null,
  lastSyncAt: null,
  prListRevision: 0,
  commentsRevision: 0,
  syncStats: null,
  toasts: [],
  isOffline: false,
  authLoading: false,
  authStartRequestId: null,
  authError: null,
  authUrl: null,
  authStartedAt: null,
  authTimedOut: false,
  authInterrupted: false,
  isWsOpen: false,
  authExchange: null,
  engineStatus: null,
  appVersion: null,
  authChecked: false,
  connectedProviders: [],
  viewerLogin: '',
  repos: [],
  reposLoading: false,
  repoFilter: null,
  visiblePRIds: [],
  laneSectionsSnapshot: [],
  visiblePRBrowserUrls: [],
  visiblePRRepoKeys: [],
  collapsedRepoKeys: [],
  diffFileKeys: [],
  selectedPatchsetPair: null,
  sinceLastReviewActive: false,
  interdiffActive: false,
  threadsRevision: 0,
  inlineCommentThreadIds: [],
  reviewDrafts: [],
  reviewDraftsPrId: null,
  lineComment: null,
  activeInlineCommentThreadId: null,
  replyingInlineCommentThreadId: null,
  hideWhitespaceHunks: false,
  isRecordingKeybinding: false,
  diffCollapseOverrides: [],
  commentMinSeverity: null,
  commentHumanOnly: false,
  dismissBatches: [],
  byokSessionTokens: 0,
  byokEstCost: 0,
  byokAuthFailed: false,
  expertiseRepoStatuses: {},
  expertiseNudges: {},
  expertiseSettings: null,
};

export function createUIStore(): Store<UIState> {
  return new Store<UIState>(initialUIState);
}

/**
 * Toggle a PR-list repo group's collapsed state and move the cursor to the
 * group's first row (the header row when collapsing, the first PR when
 * expanding). Pure — shared by the Space keybinding and the header's onClick.
 */
export function toggleRepoGroup(state: UIState, repoKey: string): UIState {
  const isCollapsed = state.collapsedRepoKeys.includes(repoKey);
  const collapsedRepoKeys = isCollapsed
    ? state.collapsedRepoKeys.filter((k) => k !== repoKey)
    : [...state.collapsedRepoKeys, repoKey];
  // The group's first row keeps the same index before and after the toggle:
  // rows above it belong to earlier groups and are untouched.
  const headerIndex = state.visiblePRRepoKeys.indexOf(repoKey);
  return {
    ...state,
    collapsedRepoKeys,
    selectedPRIndex: headerIndex >= 0 ? headerIndex : state.selectedPRIndex,
  };
}
