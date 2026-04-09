import { useEffect, useRef, useCallback } from 'react';
import { appStore } from '../store';
import { addToast } from '../components/ToastContainer';
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

const WS_URL = 'ws://localhost:8080/ws';
const REQUEST_TIMEOUT_MS = 5000;
const BASE_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30000;
const MAX_RECONNECT_ATTEMPTS = 10;

function handleSyncComplete(data: SyncCompleteData): void {
  try {
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'ready',
      lastSyncAt: new Date().toISOString(),
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
    // Bump lastSyncAt so query subscribers know to re-fetch PR data
    appStore.setState((s) => ({
      ...s,
      lastSyncAt: data.timestamp,
    }));
  } catch {
    // Store not available (test environment)
  }
}

function handleCommentPosted(data: CommentPostedData): void {
  try {
    // Bump lastSyncAt so query subscribers know to re-fetch comment data
    appStore.setState((s) => ({
      ...s,
      lastSyncAt: new Date().toISOString(),
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
        type: data.action_type as 'approve' | 'request_changes' | 'post_comment',
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
        type: data.action_type as 'approve' | 'request_changes' | 'post_comment',
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

export function useWebSocket() {
  const wsRef = useRef<WebSocket | null>(null);
  const messageHandlersRef = useRef<Map<string, (data: unknown) => void>>(new Map());
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectAttemptRef = useRef(0);

  useEffect(() => {
    function connect() {
      log.info({ url: WS_URL }, 'attempting connection');
      try {
        appStore.setState((s) => ({ ...s, syncStatus: 'connecting' }));
      } catch {
        log.warn('store not available — skipping connection');
        return;
      }

      const ws = new WebSocket(WS_URL);

      ws.onopen = () => {
        log.info({ url: WS_URL }, 'connected');
        reconnectAttemptRef.current = 0;
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

        // Ask Go engine for current auth status
        const checkMsg: WSMessage = {
          id: `req-${Date.now()}-${Math.random()}`,
          type: 'AUTH_CHECK',
          data: {},
          timestamp: new Date().toISOString(),
        };
        log.info({ id: checkMsg.id }, 'sending AUTH_CHECK');
        ws.send(JSON.stringify(checkMsg));
      };

      ws.onmessage = (event: MessageEvent<string>) => {
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
          case 'PR_APPROVED':
            handlePRApproved(message.data as PRApprovedData);
            break;
          case 'COMMENT_POSTED':
            handleCommentPosted(message.data as CommentPostedData);
            break;
          case 'SYNC_ERROR':
            handleSyncError(message.data as SyncErrorData);
            addToast({ type: 'error', message: formatErrorMessage((message.data as SyncErrorData).error_code), dismissible: true });
            break;
          case 'AUTH_TOKEN_EXPIRED':
            handleAuthExpired(message.data as AuthExpiredData);
            addToast({ type: 'error', message: formatErrorMessage('AUTH_TOKEN_EXPIRED'), dismissible: true });
            break;
          case 'RATE_LIMITED':
            handleRateLimited(message.data as RateLimitedData);
            break;
          case 'ACTION_COMPLETE':
            handleActionComplete(message.data as ActionCompleteData);
            break;
          case 'ACTION_FAILED':
            handleActionFailed(message.data as ActionFailedData);
            addToast({ type: 'error', message: formatErrorMessage((message.data as ActionFailedData).error), dismissible: true });
            break;
          case 'AUTH_REDIRECT': {
            const payload = message.data as { provider: string; url: string };
            try {
              if (window.electron?.openExternal) {
                void window.electron.openExternal(payload.url);
              }
              appStore.setState((s) => ({
                ...s,
                authLoading: true,
                authError: null,
              }));
            } catch { /* test env */ }
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
            } catch { /* test env */ }
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
            } catch { /* test env */ }
            break;
          }
          case 'AUTH_SUCCESS': {
            const payload = message.data as { provider: string; username: string };
            try {
              appStore.setState((s) => ({
                ...s,
                isAuthenticated: true,
                authChecked: true,
                authLoading: false,
                authError: null,
                connectedProviders: [
                  ...s.connectedProviders.filter((p) => p.provider !== payload.provider),
                  { provider: payload.provider, username: payload.username ?? '' },
                ],
              }));
            } catch { /* test env */ }
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
            } catch { /* test env */ }
            addToast({ type: 'error', message: payload.error, dismissible: true });
            break;
          }
          case 'DIFF_READY':
          case 'COMMENTS_READY': {
            const handler = messageHandlersRef.current.get(message.id);
            if (handler) {
              handler(message.data);
              messageHandlersRef.current.delete(message.id);
            }
            break;
          }
          default:
            log.warn({ type: message.type }, 'unhandled event type');
        }
      };

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
        const isOnline = typeof navigator !== 'undefined' ? navigator.onLine : true;
        try {
          appStore.setState((s) => ({
            ...s,
            syncStatus: 'disconnected',
            isOffline: !isOnline,
          }));
        } catch {
          // Store not available (test environment)
        }

        if (reconnectAttemptRef.current < MAX_RECONNECT_ATTEMPTS) {
          const delay = Math.min(
            BASE_RECONNECT_DELAY_MS * Math.pow(2, reconnectAttemptRef.current),
            MAX_RECONNECT_DELAY_MS
          );
          reconnectAttemptRef.current += 1;
          log.info({ delay, attempt: reconnectAttemptRef.current, maxAttempts: MAX_RECONNECT_ATTEMPTS }, 'scheduling reconnect');
          reconnectTimerRef.current = setTimeout(() => {
            connect();
          }, delay);
        } else {
          log.error({ maxAttempts: MAX_RECONNECT_ATTEMPTS }, 'max reconnect attempts reached');
          try {
            appStore.setState((s) => ({
              ...s,
              syncStatus: 'error',
              syncError: 'Connection lost. Please restart the app.',
            }));
          } catch {
            // Store not available (test environment)
          }
        }
      };

      wsRef.current = ws;
    }

    connect();

    return () => {
      if (reconnectTimerRef.current !== null) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, []);

  const send = useCallback((type: string, data: unknown): string | undefined => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      const state = ws ? ws.readyState : 'null';
      log.error({ type, ref: ws ? 'exists' : 'null', readyState: state, expected: WebSocket.OPEN }, 'send failed — not connected');
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
  }, []);

  const request = useCallback(
    (type: string, data: unknown): Promise<unknown> => {
      return new Promise((resolve, reject) => {
        const id = send(type, data);
        if (id === undefined) {
          reject(new Error('WebSocket not connected'));
          return;
        }

        const timeout = setTimeout(() => {
          messageHandlersRef.current.delete(id);
          reject(new Error(`Timeout waiting for ${type} response`));
        }, REQUEST_TIMEOUT_MS);

        messageHandlersRef.current.set(id, (responseData) => {
          clearTimeout(timeout);
          resolve(responseData);
        });
      });
    },
    [send]
  );

  return { send, request, wsRef };
}
