import { useCallback, useEffect } from 'react';
import { addToast } from '../components/ToastContainer';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type {
  ExpertiseNudgePayload,
  ExpertiseRepoStatus,
  ExpertiseSettingsPayload,
  RepoInfo,
} from '../types';
import type {
  LegacyScanDevFolderResult,
  LocationError,
  ScanDevFolderResult,
} from '../types/repoLocation';
import { formatErrorMessage } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('WebSocket');

interface WSMessage {
  id: string;
  type: string;
  data?: unknown;
  timestamp: string;
}

interface SyncCompleteData {
  pr_count: number;
  new_prs: number;
  updated_prs: number;
  deleted_prs: number;
  duration_ms: number;
}

interface PRApprovedData {
  pr_id: string;
  repo: string;
  number: number;
  approved_by: string;
  timestamp: string;
}

interface CommentPostedData {
  pr_id: string;
  comment_id: string;
  author: string;
  body: string;
  line?: number;
  created_at: string;
}

interface SyncErrorData {
  error_code: string;
  message: string;
}

interface AuthExpiredData {
  provider: string;
  message: string;
}

interface RateLimitedData {
  reset_at: string;
}

interface ActionCompleteData {
  action_type: string;
  pr_id: string;
  message: string;
}

interface ActionFailedData {
  action_type: string;
  pr_id: string;
  error: string;
}

interface ThreadStateChangedData {
  pr_id: string;
  thread_id: string;
}

interface PRsReadChangedData {
  pr_ids: string[];
}

interface ScanDevFolderRequest {
  path: string;
  reason: 'startup';
}

type LocationErrorData = LocationError;
type ScanDevFolderData = ScanDevFolderResult;

interface ExpertiseProgressData {
  repo_id: string;
  files_done: number;
  files_total: number;
}

interface ExpertiseReadyData {
  repo_id: string;
  indexed_sha: string;
  files_total: number;
  skipped_files: number;
}

interface ExpertiseErrorData {
  repo_id: string;
  code: string;
  message: string;
}

const LOCATION_ERROR_CODES = [
  'path_missing',
  'remote_mismatch',
  'permission_denied',
  'invalid_url',
  'invalid_path',
  'destination_required',
  'destination_conflict',
  'clone_failed',
  'auth_failed',
  'cancelled',
  'delete_failed',
  'scan_partial',
] as const;

const LOCATION_ERROR_OPERATIONS = [
  'scan',
  'fetch',
  'diff',
  'clone',
  'prune',
  'recover',
] as const;

const DEFAULT_WS_PORT = 8080;

/**
 * Resolves the sync-engine WebSocket URL. The Go process picks its port
 * dynamically; the preload script exposes it synchronously as
 * `window.electron.wsPort`. Falls back to 8080 outside Electron (tests,
 * plain browser dev).
 */
function getWsUrl(): string {
  const exposedPort =
    typeof window !== 'undefined' ? window.electron?.wsPort : undefined;
  const port =
    typeof exposedPort === 'number' &&
    Number.isInteger(exposedPort) &&
    exposedPort > 0 &&
    exposedPort <= 65535
      ? exposedPort
      : DEFAULT_WS_PORT;
  return `ws://localhost:${port}/ws`;
}
const REQUEST_TIMEOUT_MS = 5000;
const PR_SYNC_RESPONSE_MARGIN_MS = 15000;
const PR_SYNC_REQUEST_TIMEOUT_MS = 2 * 60 * 1000 + PR_SYNC_RESPONSE_MARGIN_MS;
const BASE_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30000;
const MAX_RECONNECT_ATTEMPTS = 10;

// ── Singleton WebSocket manager ──────────────────────────────────────
let singletonWs: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempt = 0;
let refCount = 0;
let hasSentStartupDevFolderScan = false;
const messageHandlers = new Map<string, (data: unknown) => void>();

function handleSyncComplete(data: SyncCompleteData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'ready',
      lastSyncAt: new Date().toISOString(),
      prListRevision: s.prListRevision + 1,
      syncStats: {
        prCount: data.pr_count,
        newPRs: data.new_prs,
        updatedPRs: data.updated_prs,
      },
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handlePRApproved(data: PRApprovedData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      lastSyncAt: data.timestamp,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handleCommentPosted(_data: CommentPostedData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      lastSyncAt: new Date().toISOString(),
      commentsRevision: s.commentsRevision + 1,
      threadsRevision: s.threadsRevision + 1,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handleSyncError(data: SyncErrorData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'error',
      syncError: `${data.error_code}: ${data.message}`,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handleAuthExpired(data: AuthExpiredData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'error',
      syncError: `Auth expired (${data.provider}): ${data.message}`,
      isAuthenticated: false,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handleRateLimited(data: RateLimitedData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'rate_limited',
      syncError: `Rate limited until ${data.reset_at}`,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handleActionComplete(data: ActionCompleteData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      pendingAction: null,
      actionResult: {
        type: data.action_type as
          | 'approve'
          | 'request_changes'
          | 'post_comment'
          | 'reply_comment',
        prId: data.pr_id,
        success: true,
        message: data.message,
      },
      confirmingAction: null,
      lastSyncAt: new Date().toISOString(),
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handleActionFailed(data: ActionFailedData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      pendingAction: null,
      actionResult: {
        type: data.action_type as
          | 'approve'
          | 'request_changes'
          | 'post_comment'
          | 'reply_comment',
        prId: data.pr_id,
        success: false,
        message: data.error,
      },
      confirmingAction: null,
    }));
  } catch {
    // Store not available (test environment)
  }
}

/**
 * A thread's resolved/reopened state changed on the server (our write landed,
 * or a teammate changed it). Bump `threadsRevision` so the open threads view
 * and merge-readiness badge re-fetch — no full sync cycle needed.
 */
function handleThreadStateChanged(_data: ThreadStateChangedData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      threadsRevision: s.threadsRevision + 1,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function bumpPRListRevision(): void {
  try {
    appStore.setState((s) => ({
      ...s,
      prListRevision: s.prListRevision + 1,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handlePRMetadataChanged(_data: unknown): void {
  bumpPRListRevision();
}

function handlePRsReadChanged(_data: PRsReadChangedData): void {
  bumpPRListRevision();
}

function handleCIStatusChanged(_data: unknown): void {
  try {
    appStore.setState((s) => ({
      ...s,
      prListRevision: s.prListRevision + 1,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function mergeExpertiseStatus(status: ExpertiseRepoStatus): void {
  appStore.setState((s) => ({
    ...s,
    expertiseRepoStatuses: {
      ...s.expertiseRepoStatuses,
      [status.repo_id]: {
        ...(s.expertiseRepoStatuses[status.repo_id] ?? {}),
        ...status,
      },
    },
  }));
}

function handleExpertiseIndexProgress(data: ExpertiseProgressData): void {
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'indexing',
    files_done: data.files_done,
    files_total: data.files_total,
  });
}

function handleExpertiseReady(data: ExpertiseReadyData): void {
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'ready',
    indexed_sha: data.indexed_sha,
    files_total: data.files_total,
    skipped_files: data.skipped_files,
  });
  appStore.setState((s) => ({
    ...s,
    prListRevision: s.prListRevision + 1,
  }));
}

function handleExpertiseError(data: ExpertiseErrorData): void {
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'error',
    code: data.code,
    message: data.message,
  });
  addToast({
    type: 'error',
    message: `Expertise indexing failed: ${data.message}`,
    dismissible: true,
  });
}

function handleExpertiseNudge(data: ExpertiseNudgePayload): void {
  appStore.setState((s) => ({
    ...s,
    expertiseNudges: {
      ...s.expertiseNudges,
      [data.repo_id]: data,
    },
  }));
}

function handleExpertiseSettings(data: ExpertiseSettingsPayload): void {
  appStore.setState((s) => ({
    ...s,
    expertiseSettings: data,
  }));
}

function isRecord(data: unknown): data is Record<string, unknown> {
  return typeof data === 'object' && data !== null;
}

function isLocationError(data: unknown): data is LocationErrorData {
  if (!isRecord(data)) {
    return false;
  }

  return (
    typeof data.repo_id === 'string' &&
    LOCATION_ERROR_OPERATIONS.includes(
      data.operation as (typeof LOCATION_ERROR_OPERATIONS)[number],
    ) &&
    LOCATION_ERROR_CODES.includes(
      data.code as (typeof LOCATION_ERROR_CODES)[number],
    ) &&
    typeof data.message === 'string'
  );
}

function isScannedRepository(data: unknown): boolean {
  if (!isRecord(data)) {
    return false;
  }

  return (
    typeof data.repo_id === 'string' &&
    typeof data.path === 'string' &&
    typeof data.remote_url === 'string' &&
    data.source === 'local'
  );
}

function isLegacyScanDevFolderResult(
  data: unknown,
): data is LegacyScanDevFolderResult {
  if (!isRecord(data)) {
    return false;
  }

  return (
    typeof data.path === 'string' &&
    Array.isArray(data.repositories) &&
    data.repositories.every(isScannedRepository) &&
    Array.isArray(data.errors) &&
    data.errors.every(isLocationError)
  );
}

function isScanDevFolderResult(data: unknown): data is ScanDevFolderData {
  if (isLegacyScanDevFolderResult(data)) {
    return true;
  }

  if (!isRecord(data)) {
    return false;
  }

  return (
    data.ok === true &&
    typeof data.discovered === 'number' &&
    typeof data.healthy === 'number' &&
    typeof data.degraded === 'number' &&
    typeof data.skipped === 'number' &&
    typeof data.partial === 'boolean'
  );
}

function handleScanDevFolderResult(data: unknown): ScanDevFolderData | null {
  if (!isScanDevFolderResult(data)) {
    log.warn({ data }, 'invalid SCAN_DEV_FOLDER_RESULT payload');
    return null;
  }

  const payload = data;
  if (!isLegacyScanDevFolderResult(payload)) {
    return payload;
  }

  if (payload.errors.length === 0) {
    return payload;
  }

  log.warn(
    { path: payload.path, errors: payload.errors.length },
    'dev-folder scan completed with location errors',
  );
  return payload;
}

function handleLocationError(data: unknown): void {
  if (!isLocationError(data)) {
    log.warn({ data }, 'invalid LOCATION_ERROR payload');
    return;
  }

  log.warn(
    { repoId: data.repo_id, operation: data.operation, code: data.code },
    data.message,
  );
}

function onMessage(event: MessageEvent<string>): void {
  let message: WSMessage;
  try {
    message = JSON.parse(event.data) as WSMessage;
  } catch (error) {
    log.error({ err: error }, 'message parse error');
    return;
  }

  log.debug({ type: message.type, data: message.data }, 'received event');

  switch (message.type) {
    case 'SYNC_COMPLETE':
      handleSyncComplete(message.data as SyncCompleteData);
      break;
    case 'SYNC_TRIGGERED': {
      const queued = (message.data as { queued?: boolean }).queued === true;
      if (queued) {
        try {
          appStore.setState((s) => ({ ...s, syncStatus: 'syncing' }));
        } catch {
          // Store not available (test environment)
        }
      } else {
        log.warn('manual sync not queued — sync already in progress');
      }
      break;
    }
    case 'PR_APPROVED':
      handlePRApproved(message.data as PRApprovedData);
      break;
    case 'COMMENT_POSTED':
      handleCommentPosted(message.data as CommentPostedData);
      break;
    case 'SYNC_ERROR':
      handleSyncError(message.data as SyncErrorData);
      addToast({
        type: 'error',
        message: formatErrorMessage((message.data as SyncErrorData).error_code),
        dismissible: true,
      });
      break;
    case 'AUTH_TOKEN_EXPIRED':
      handleAuthExpired(message.data as AuthExpiredData);
      addToast({
        type: 'error',
        message: formatErrorMessage('AUTH_TOKEN_EXPIRED'),
        dismissible: true,
      });
      break;
    case 'RATE_LIMITED':
      handleRateLimited(message.data as RateLimitedData);
      break;
    case 'ACTION_COMPLETE':
      handleActionComplete(message.data as ActionCompleteData);
      break;
    case 'ACTION_FAILED':
      handleActionFailed(message.data as ActionFailedData);
      addToast({
        type: 'error',
        message: formatErrorMessage((message.data as ActionFailedData).error),
        dismissible: true,
      });
      break;
    case 'THREAD_RESOLVED':
    case 'THREAD_REOPENED':
      handleThreadStateChanged(message.data as ThreadStateChangedData);
      break;
    case 'PR_METADATA_CHANGED':
      handlePRMetadataChanged(message.data);
      break;
    case 'PRS_READ_CHANGED':
      handlePRsReadChanged(message.data as PRsReadChangedData);
      break;
    case 'CI_STATUS_CHANGED':
      handleCIStatusChanged(message.data);
      break;
    case 'SCAN_DEV_FOLDER_RESULT': {
      const payload = handleScanDevFolderResult(message.data);
      if (payload === null) {
        break;
      }
      const scanHandler = messageHandlers.get(message.id);
      if (scanHandler) {
        scanHandler(payload);
        messageHandlers.delete(message.id);
      }
      break;
    }
    case 'LOCATION_ERROR':
      handleLocationError(message.data);
      break;
    case 'EXPERTISE_INDEX_PROGRESS':
      handleExpertiseIndexProgress(message.data as ExpertiseProgressData);
      break;
    case 'EXPERTISE_READY':
      handleExpertiseReady(message.data as ExpertiseReadyData);
      break;
    case 'EXPERTISE_ERROR': {
      const payload = message.data as ExpertiseErrorData;
      handleExpertiseError(payload);
      const handler = messageHandlers.get(message.id);
      if (handler) {
        handler({ error: payload.message || payload.code });
        messageHandlers.delete(message.id);
      }
      break;
    }
    case 'EXPERTISE_NUDGE':
      handleExpertiseNudge(message.data as ExpertiseNudgePayload);
      break;
    case 'EXPERTISE_DATA':
    case 'EXPERTISE_ENABLE_ACCEPTED':
    case 'EXPERTISE_DISABLE_ACCEPTED':
    case 'EXPERTISE_NUDGE_DISMISSED': {
      const payload = message.data;
      const handler = messageHandlers.get(message.id);
      if (handler) {
        handler(payload);
        messageHandlers.delete(message.id);
      }
      break;
    }
    case 'EXPERTISE_SETTINGS':
    case 'EXPERTISE_SETTINGS_UPDATED': {
      const payload = message.data as ExpertiseSettingsPayload;
      handleExpertiseSettings(payload);
      const handler = messageHandlers.get(message.id);
      if (handler) {
        handler(payload);
        messageHandlers.delete(message.id);
      }
      break;
    }
    case 'AUTH_REDIRECT': {
      const payload = message.data as { provider: string; url: string };
      try {
        if (!window.electron?.openExternal) {
          appStore.setState((s) => ({
            ...s,
            authLoading: false,
            authError: 'Browser launch is unavailable.',
          }));
          break;
        }

        void window.electron.openExternal(payload.url).then((result) => {
          if (result && 'err' in result) {
            appStore.setState((s) => ({
              ...s,
              authLoading: false,
              authError: (result as { err: string }).err,
            }));
            addToast({
              type: 'error',
              message: (result as { err: string }).err,
              dismissible: true,
            });
            return;
          }
          appStore.setState((s) => ({
            ...s,
            authLoading: true,
            authError: null,
          }));
        });
      } catch {
        /* test env */
      }
      break;
    }
    case 'AUTH_CHECK_RESULT': {
      const payload = message.data as {
        authenticated: boolean;
        providers: Array<{ provider: string; username: string }>;
      };
      try {
        appStore.setState((s) => ({
          ...s,
          authChecked: true,
          authLoading: false,
          isAuthenticated: payload.authenticated,
          connectedProviders: payload.providers ?? [],
        }));
      } catch {
        /* test env */
      }
      break;
    }
    case 'AUTH_DISCONNECT_RESULT': {
      const payload = message.data as {
        authenticated: boolean;
        providers: Array<{ provider: string; username: string }>;
      };
      try {
        appStore.setState((s) => ({
          ...s,
          isAuthenticated: payload.authenticated,
          connectedProviders: payload.providers ?? [],
        }));
      } catch {
        /* test env */
      }
      break;
    }
    case 'AUTH_SUCCESS': {
      const payload = message.data as {
        provider: string;
        username: string;
      };
      try {
        appStore.setState((s) => ({
          ...s,
          isAuthenticated: true,
          authChecked: true,
          authLoading: false,
          authError: null,
          connectedProviders: [
            ...s.connectedProviders.filter(
              (p) => p.provider !== payload.provider,
            ),
            {
              provider: payload.provider,
              username: payload.username ?? '',
            },
          ],
        }));
      } catch {
        /* test env */
      }
      break;
    }
    case 'AUTH_FAILED': {
      const payload = message.data as { error: string };
      try {
        appStore.setState((s) => ({
          ...s,
          authLoading: false,
          authError: payload.error,
        }));
      } catch {
        /* test env */
      }
      addToast({
        type: 'error',
        message: payload.error,
        dismissible: true,
      });
      break;
    }
    case 'COMMENTS_READY': {
      const handler = messageHandlers.get(message.id);
      if (handler) {
        handler(message.data);
        messageHandlers.delete(message.id);
        break;
      }
      // Server-initiated push (async LLM tier upgraded classifications):
      // no matching request id — bump commentsRevision so open views refetch.
      try {
        appStore.setState((s) => ({
          ...s,
          commentsRevision: s.commentsRevision + 1,
        }));
      } catch {
        /* test env */
      }
      break;
    }
    case 'BYOK_USAGE': {
      const payload = message.data as {
        session_tokens?: number;
        est_cost?: number;
      };
      try {
        appStore.setState((s) => ({
          ...s,
          byokSessionTokens: payload.session_tokens ?? 0,
          byokEstCost: payload.est_cost ?? 0,
        }));
      } catch {
        /* test env */
      }
      break;
    }
    case 'BYOK_AUTH_FAILED': {
      try {
        appStore.setState((s) => ({ ...s, byokAuthFailed: true }));
      } catch {
        /* test env */
      }
      addToast({
        type: 'error',
        message: 'AI classification: API key rejected — check Settings',
        dismissible: true,
      });
      break;
    }
    case 'PR_SYNCED':
    case 'PRS_READ_ACK':
    case 'DIFF_READY':
    case 'PATCHSETS_READY':
    case 'RANGE_DIFF_READY':
    case 'FILE_VIEWED_ACK':
    case 'REVIEW_THREADS_READY':
    case 'ACTION_RESULT':
    case 'VALIDATE_DEV_FOLDER_RESULT':
    case 'ADD_REPO_BY_URL_RESULT':
    case 'GET_CACHE_STATS_RESULT':
    case 'SET_EXPERTISE_SETTINGS':
    case 'PRUNE_CACHE_RESULT':
    case 'CLEAR_CACHE_RESULT':
    case 'DELETE_CACHE_REPO_RESULT':
    case 'RECOVER_REPO_PATH_RESULT':
    case 'RECOVER_REPO_TO_CACHE_RESULT':
    case 'REMOVE_REPO_PREFLIGHT_RESULT':
    case 'REMOVE_REPO_RESULT':
    case 'CLASSIFICATION_SETTINGS_UPDATED':
    case 'BYOK_KEY_UPDATED':
    case 'DISMISS_RESULT':
    case 'UNDISMISS_RESULT':
    case 'PR_LIST_READY': {
      const handler = messageHandlers.get(message.id);
      if (handler) {
        handler(message.data);
        messageHandlers.delete(message.id);
      }
      break;
    }
    case 'REPO_LIST_READY': {
      const payload = message.data as {
        repos?: RepoInfo[];
        error?: string;
      };
      try {
        if (payload.error) {
          appStore.setState((s) => ({
            ...s,
            reposLoading: false,
          }));
          addToast({
            type: 'error',
            message: `Failed to load repos: ${payload.error}`,
            dismissible: true,
          });
        } else {
          appStore.setState((s) => ({
            ...s,
            repos: payload.repos ?? [],
            reposLoading: false,
          }));
        }
      } catch {
        /* test env */
      }
      const repoListHandler = messageHandlers.get(message.id);
      if (repoListHandler) {
        repoListHandler(message.data);
        messageHandlers.delete(message.id);
      }
      break;
    }
    case 'WATCH_REPO_RESULT': {
      const payload = message.data as { repo_id: string; watched: boolean };
      try {
        appStore.setState((s) => ({
          ...s,
          repos: s.repos.map((r) =>
            r.id === payload.repo_id ? { ...r, watched: payload.watched } : r,
          ),
        }));
      } catch {
        /* test env */
      }
      const watchHandler = messageHandlers.get(message.id);
      if (watchHandler) {
        watchHandler(message.data);
        messageHandlers.delete(message.id);
      }
      break;
    }
    default:
      log.warn({ type: message.type }, 'unhandled event type');
  }
}

function sendStartupDevFolderScan(ws: WebSocket): void {
  const path = settingsStore.state.repositories.devFolderPath;
  if (hasSentStartupDevFolderScan || !path) {
    return;
  }

  hasSentStartupDevFolderScan = true;

  const data: ScanDevFolderRequest = { path, reason: 'startup' };
  const scanMsg: WSMessage = {
    id: `req-${Date.now()}-${Math.random()}`,
    type: 'SCAN_DEV_FOLDER',
    data,
    timestamp: new Date().toISOString(),
  };
  log.info({ id: scanMsg.id }, 'sending SCAN_DEV_FOLDER');
  ws.send(JSON.stringify(scanMsg));
}

function connect(): void {
  if (singletonWs && singletonWs.readyState <= WebSocket.OPEN) {
    return;
  }

  const wsUrl = getWsUrl();
  log.info({ url: wsUrl }, 'attempting connection');
  try {
    appStore.setState((s) => ({ ...s, syncStatus: 'connecting' }));
  } catch {
    log.warn('store not available — skipping connection');
    return;
  }

  const ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    log.info({ url: wsUrl }, 'connected');
    reconnectAttempt = 0;
    try {
      appStore.setState((s) => ({
        ...s,
        syncStatus: 'connected',
        syncError: null,
        lastSyncAt: new Date().toISOString(),
        isOffline: false,
        authChecked: false,
      }));
    } catch {
      // Store not available (test environment)
    }

    const checkMsg: WSMessage = {
      id: `req-${Date.now()}-${Math.random()}`,
      type: 'AUTH_CHECK',
      data: {},
      timestamp: new Date().toISOString(),
    };
    log.info({ id: checkMsg.id }, 'sending AUTH_CHECK');
    ws.send(JSON.stringify(checkMsg));
    sendStartupDevFolderScan(ws);
  };

  ws.onmessage = onMessage;

  ws.onerror = (error) => {
    log.error({ err: error }, 'connection error');
    try {
      appStore.setState((s) => ({
        ...s,
        syncStatus: 'error',
        syncError: 'WebSocket connection failed',
      }));
    } catch {
      // Store not available (test environment)
    }
  };

  ws.onclose = () => {
    log.info('disconnected');
    singletonWs = null;
    // Each new connection should re-run the startup dev-folder scan;
    // disconnect() resets this too, but dropped sockets reconnect directly.
    hasSentStartupDevFolderScan = false;
    const isOnline = typeof navigator !== 'undefined' ? navigator.onLine : true;
    try {
      appStore.setState((s) => ({
        ...s,
        syncStatus: 'disconnected',
        isOffline: !isOnline,
        authLoading: false,
      }));
    } catch {
      // Store not available (test environment)
    }

    if (refCount > 0 && reconnectAttempt < MAX_RECONNECT_ATTEMPTS) {
      const delay = Math.min(
        BASE_RECONNECT_DELAY_MS * 2 ** reconnectAttempt,
        MAX_RECONNECT_DELAY_MS,
      );
      reconnectAttempt += 1;
      log.info(
        {
          delay,
          attempt: reconnectAttempt,
          maxAttempts: MAX_RECONNECT_ATTEMPTS,
        },
        'scheduling reconnect',
      );
      reconnectTimer = setTimeout(connect, delay);
    } else if (reconnectAttempt >= MAX_RECONNECT_ATTEMPTS) {
      log.error(
        { maxAttempts: MAX_RECONNECT_ATTEMPTS },
        'max reconnect attempts reached',
      );
      try {
        appStore.setState((s) => ({
          ...s,
          syncStatus: 'error',
          syncError: 'Connection lost. Please restart the app.',
          authChecked: true,
          isAuthenticated: false,
        }));
      } catch {
        // Store not available (test environment)
      }
    }
  };

  singletonWs = ws;
}

function disconnect(): void {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  singletonWs?.close();
  singletonWs = null;
  reconnectAttempt = 0;
  hasSentStartupDevFolderScan = false;
}

function sendMessage(type: string, data: unknown): string | undefined {
  const ws = singletonWs;
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    const state = ws ? ws.readyState : 'null';
    log.error(
      {
        type,
        ref: ws ? 'exists' : 'null',
        readyState: state,
        expected: WebSocket.OPEN,
      },
      'send failed — not connected',
    );
    log.error({ type, data }, 'dropped message');
    return undefined;
  }

  const id = `req-${Date.now()}-${Math.random()}`;
  const message: WSMessage = {
    id,
    type,
    data,
    timestamp: new Date().toISOString(),
  };

  log.info({ type, id }, 'sending message');
  ws.send(JSON.stringify(message));
  return id;
}

/**
 * Extracts a non-empty `error` string from a response payload, or null.
 * Go handlers report recoverable failures (e.g. "diff not synced yet") as
 * an `error` field inside otherwise-successful *_READY envelopes.
 */
export function extractResponseError(data: unknown): string | null {
  if (typeof data !== 'object' || data === null) {
    return null;
  }
  const err = (data as { error?: unknown }).error;
  return typeof err === 'string' && err.length > 0 ? err : null;
}

const CONNECT_WAIT_INTERVAL_MS = 250;

function requestTimeoutFor(type: string): number {
  return type === 'REQUEST_PR_SYNC'
    ? PR_SYNC_REQUEST_TIMEOUT_MS
    : REQUEST_TIMEOUT_MS;
}

function requestMessage(type: string, data: unknown): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const requestTimeout = requestTimeoutFor(type);
    const connectDeadline = Date.now() + REQUEST_TIMEOUT_MS;

    // The socket connects asynchronously on mount; requests issued in the
    // same render tick would otherwise be dropped. Wait briefly for OPEN.
    const trySend = (): void => {
      if (singletonWs === null || singletonWs.readyState !== WebSocket.OPEN) {
        if (Date.now() >= connectDeadline) {
          reject(new Error(`WebSocket not connected (sending ${type})`));
          return;
        }
        setTimeout(trySend, CONNECT_WAIT_INTERVAL_MS);
        return;
      }

      const id = sendMessage(type, data);
      if (id === undefined) {
        reject(new Error('WebSocket not connected'));
        return;
      }

      const timeout = setTimeout(() => {
        messageHandlers.delete(id);
        const readyState = singletonWs?.readyState ?? 'no-socket';
        log.error(
          { type, id, readyState, pendingRequests: messageHandlers.size },
          'request timed out',
        );
        reject(
          new Error(
            `Timeout waiting for ${type} response (socket state: ${readyState} — see bistro-sync.log for the backend side)`,
          ),
        );
      }, requestTimeout);

      messageHandlers.set(id, (responseData) => {
        clearTimeout(timeout);
        const responseError = extractResponseError(responseData);
        if (responseError !== null) {
          reject(new Error(responseError));
          return;
        }
        resolve(responseData);
      });
    };

    trySend();
  });
}

// ── Hook (ref-counted, shares singleton) ─────────────────────────────
export function useWebSocket() {
  useEffect(() => {
    refCount += 1;
    if (refCount === 1) {
      connect();
    }
    return () => {
      refCount -= 1;
      if (refCount === 0) {
        disconnect();
      }
    };
  }, []);

  const send = useCallback(
    (type: string, data: unknown): string | undefined =>
      sendMessage(type, data),
    [],
  );

  const request = useCallback(
    (type: string, data: unknown): Promise<unknown> =>
      requestMessage(type, data),
    [],
  );

  return { send, request };
}
