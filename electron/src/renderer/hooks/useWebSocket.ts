import { useCallback, useEffect } from 'react';
import { addToast } from '../components/ToastContainer';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { RepoInfo } from '../types';
import type { LocationError, ScanDevFolderResult } from '../types/repoLocation';
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

interface ScanDevFolderRequest {
  path: string;
  reason: 'startup';
}

type LocationErrorData = LocationError;
type ScanDevFolderData = ScanDevFolderResult;

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

function handlePRMetadataChanged(_data: unknown): void {
  try {
    appStore.setState((s) => ({
      ...s,
      prListRevision: s.prListRevision + 1,
    }));
  } catch {
    // Store not available (test environment)
  }
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

function handleScanDevFolderResult(data: ScanDevFolderData): void {
  if (data.errors.length === 0) {
    return;
  }

  log.warn(
    { path: data.path, errors: data.errors.length },
    'dev-folder scan completed with location errors',
  );
}

function handleLocationError(data: LocationErrorData): void {
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
    case 'CI_STATUS_CHANGED':
      handleCIStatusChanged(message.data);
      break;
    case 'SCAN_DEV_FOLDER_RESULT':
      handleScanDevFolderResult(message.data as ScanDevFolderData);
      break;
    case 'LOCATION_ERROR':
      handleLocationError(message.data as LocationErrorData);
      break;
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
      if (singletonWs?.readyState === WebSocket.OPEN) {
        sendStartupDevFolderScan(singletonWs);
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
    case 'DIFF_READY':
    case 'COMMENTS_READY':
    case 'PATCHSETS_READY':
    case 'RANGE_DIFF_READY':
    case 'FILE_VIEWED_ACK':
    case 'REVIEW_THREADS_READY':
    case 'ACTION_RESULT':
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

function requestMessage(type: string, data: unknown): Promise<unknown> {
  return new Promise((resolve, reject) => {
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
      }, REQUEST_TIMEOUT_MS);

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
