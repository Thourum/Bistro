import { useEffect, useRef, useCallback } from 'react';
import { appStore } from '../store';

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

const WS_URL = 'ws://localhost:8080';
const REQUEST_TIMEOUT_MS = 5000;
const RECONNECT_DELAY_MS = 3000;

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

  useEffect(() => {
    function connect() {
      try {
        appStore.setState((s) => ({ ...s, syncStatus: 'connecting' }));
      } catch {
        // Store not available (e.g., in test environment)
        return;
      }

      const ws = new WebSocket(WS_URL);

      ws.onopen = () => {
        console.log('WebSocket connected');
        try {
          appStore.setState((s) => ({
            ...s,
            syncStatus: 'connected',
            syncError: null,
            lastSyncAt: new Date().toISOString(),
          }));
        } catch {
          // Store not available (test environment)
        }
      };

      ws.onmessage = (event: MessageEvent<string>) => {
        let message: WSMessage;
        try {
          message = JSON.parse(event.data) as WSMessage;
        } catch (error) {
          console.error('WebSocket parse error:', error);
          return;
        }

        console.log('WebSocket event:', message.type, message.data);

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
            break;
          case 'AUTH_TOKEN_EXPIRED':
            handleAuthExpired(message.data as AuthExpiredData);
            break;
          case 'RATE_LIMITED':
            handleRateLimited(message.data as RateLimitedData);
            break;
          case 'ACTION_COMPLETE':
            handleActionComplete(message.data as ActionCompleteData);
            break;
          case 'ACTION_FAILED':
            handleActionFailed(message.data as ActionFailedData);
            break;
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
            console.warn('Unhandled WebSocket event type:', message.type);
        }
      };

      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
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
        console.log('WebSocket disconnected');
        try {
          appStore.setState((s) => ({ ...s, syncStatus: 'disconnected' }));
        } catch {
          // Store not available (test environment)
        }

        reconnectTimerRef.current = setTimeout(() => {
          console.log('Attempting WebSocket reconnect...');
          connect();
        }, RECONNECT_DELAY_MS);
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
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {
      console.error('WebSocket not connected');
      return undefined;
    }

    const id = `req-${Date.now()}-${Math.random()}`;
    const message: WSMessage = {
      id,
      type,
      data,
      timestamp: new Date().toISOString(),
    };

    wsRef.current.send(JSON.stringify(message));
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
