import { err, fromThrowable, ok, type Result, ResultAsync } from 'neverthrow';
import { useCallback, useEffect } from 'react';
import { z } from 'zod';
import { addToast } from '../components/ToastContainer';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { ExpertiseRepoStatus } from '../types';
import { formatErrorMessage } from '../utils/error';
import { createLogger } from '../utils/logger';

const log = createLogger('WebSocket');

const messageEnvelopeSchema = z.object({
  id: z.string().optional(),
  type: z.string(),
  data: z.unknown().optional(),
  timestamp: z.string().optional(),
});

type WSMessage = z.infer<typeof messageEnvelopeSchema>;

interface ScanDevFolderRequest {
  path: string;
  reason: 'startup';
}

const locationErrorSchema = z.object({
  repo_id: z.string(),
  operation: z.enum(['scan', 'fetch', 'diff', 'clone', 'prune', 'recover']),
  code: z.enum([
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
  ]),
  message: z.string(),
});
const scanDevFolderResultSchema = z.union([
  z.object({
    path: z.string(),
    repositories: z.array(
      z.object({
        repo_id: z.string(),
        path: z.string(),
        remote_url: z.string(),
        source: z.literal('local'),
      }),
    ),
    errors: z.array(locationErrorSchema),
  }),
  z.object({
    ok: z.literal(true),
    discovered: z.number(),
    healthy: z.number(),
    degraded: z.number(),
    skipped: z.number(),
    partial: z.boolean(),
  }),
]);

const dataSchemas = {
  SYNC_COMPLETE: z.object({
    pr_count: z.number().default(0),
    new_prs: z.number().default(0),
    updated_prs: z.number().default(0),
    deleted_prs: z.number().default(0),
    duration_ms: z.number().default(0),
  }),
  SYNC_TRIGGERED: z.object({ queued: z.boolean().optional() }).default({}),
  PR_APPROVED: z.object({ timestamp: z.string() }),
  COMMENT_POSTED: z.object({ pr_id: z.string() }),
  SYNC_ERROR: z.object({ error_code: z.string(), message: z.string() }),
  AUTH_TOKEN_EXPIRED: z.object({ provider: z.string(), message: z.string() }),
  RATE_LIMITED: z.object({ reset_at: z.string() }),
  ACTION_COMPLETE: z.object({
    action_type: z.enum([
      'approve',
      'request_changes',
      'post_comment',
      'reply_comment',
    ]),
    pr_id: z.string(),
    message: z.string(),
  }),
  ACTION_FAILED: z.object({
    action_type: z.enum([
      'approve',
      'request_changes',
      'post_comment',
      'reply_comment',
    ]),
    pr_id: z.string(),
    error: z.string(),
  }),
  THREAD_RESOLVED: z.object({ pr_id: z.string(), thread_id: z.string() }),
  THREAD_REOPENED: z.object({ pr_id: z.string(), thread_id: z.string() }),
  PR_METADATA_CHANGED: z.unknown(),
  PRS_READ_CHANGED: z.object({ pr_ids: z.array(z.string()) }),
  CI_STATUS_CHANGED: z.unknown(),
  SCAN_DEV_FOLDER_RESULT: scanDevFolderResultSchema,
  LOCATION_ERROR: locationErrorSchema,
  EXPERTISE_INDEX_PROGRESS: z.object({
    repo_id: z.string(),
    files_done: z.number(),
    files_total: z.number(),
  }),
  EXPERTISE_READY: z.object({
    repo_id: z.string(),
    indexed_sha: z.string(),
    files_total: z.number(),
    skipped_files: z.number(),
  }),
  EXPERTISE_ERROR: z.object({
    repo_id: z.string(),
    code: z.string(),
    message: z.string(),
  }),
  EXPERTISE_NUDGE: z.object({
    repo_id: z.string(),
    reviewed_count: z.number(),
  }),
  EXPERTISE_DATA: z.unknown(),
  EXPERTISE_ENABLE_ACCEPTED: z.unknown(),
  EXPERTISE_DISABLE_ACCEPTED: z.unknown(),
  EXPERTISE_NUDGE_DISMISSED: z.unknown(),
  EXPERTISE_SETTINGS: z.object({
    alias_emails: z.array(z.string()),
    min_surviving_lines: z.number(),
    excludes: z.record(z.string(), z.array(z.string())),
  }),
  EXPERTISE_SETTINGS_UPDATED: z.object({
    alias_emails: z.array(z.string()),
    min_surviving_lines: z.number(),
    excludes: z.record(z.string(), z.array(z.string())),
  }),
  AUTH_REDIRECT: z.object({ provider: z.string(), url: z.string().url() }),
  AUTH_CHECK_RESULT: z.object({
    authenticated: z.boolean(),
    providers: z
      .array(z.object({ provider: z.string(), username: z.string() }))
      .nullable()
      .transform((providers) => providers ?? []),
  }),
  AUTH_DISCONNECT_RESULT: z.object({
    authenticated: z.boolean(),
    providers: z
      .array(z.object({ provider: z.string(), username: z.string() }))
      .nullable()
      .transform((providers) => providers ?? []),
  }),
  AUTH_SUCCESS: z.object({ provider: z.string(), username: z.string() }),
  AUTH_FAILED: z.object({ error: z.string() }),
  COMMENTS_READY: z.unknown(),
  BYOK_USAGE: z.object({
    session_tokens: z.number().optional(),
    est_cost: z.number().optional(),
  }),
  BYOK_AUTH_FAILED: z.unknown(),
  PR_SYNCED: z.unknown(),
  PRS_READ_ACK: z.unknown(),
  DIFF_READY: z.unknown(),
  PATCHSETS_READY: z.unknown(),
  RANGE_DIFF_READY: z.unknown(),
  FILE_VIEWED_ACK: z.unknown(),
  REVIEW_THREADS_READY: z.unknown(),
  ACTION_RESULT: z.unknown(),
  PR_CHECKOUT_STATUS: z.unknown(),
  PR_CHECKOUT_RESULT: z.unknown(),
  VALIDATE_DEV_FOLDER_RESULT: z.unknown(),
  ADD_REPO_BY_URL_RESULT: z.unknown(),
  GET_CACHE_STATS_RESULT: z.unknown(),
  SET_EXPERTISE_SETTINGS: z.unknown(),
  PRUNE_CACHE_RESULT: z.unknown(),
  CLEAR_CACHE_RESULT: z.unknown(),
  DELETE_CACHE_REPO_RESULT: z.unknown(),
  RECOVER_REPO_PATH_RESULT: z.unknown(),
  RECOVER_REPO_TO_CACHE_RESULT: z.unknown(),
  REMOVE_REPO_PREFLIGHT_RESULT: z.unknown(),
  REMOVE_REPO_RESULT: z.unknown(),
  CLASSIFICATION_SETTINGS_UPDATED: z.unknown(),
  BYOK_KEY_UPDATED: z.unknown(),
  DISMISS_RESULT: z.unknown(),
  UNDISMISS_RESULT: z.unknown(),
  PR_LIST_READY: z.unknown(),
  REPO_LIST_READY: z.object({
    repos: z
      .array(
        z.object({
          id: z.string(),
          provider: z.string(),
          owner: z.string(),
          repo: z.string(),
          watched: z.boolean(),
          source: z.enum(['local', 'cache']).nullable().optional(),
          location_status: z.string().nullable().optional(),
          location_error_code: z.string().nullable().optional(),
          location_error: z.string().nullable().optional(),
        }),
      )
      .optional(),
    error: z.string().optional(),
  }),
  WATCH_REPO_RESULT: z.object({ repo_id: z.string(), watched: z.boolean() }),
} satisfies Record<string, z.ZodType>;

type WsEventType = keyof typeof dataSchemas;
type WSInboundMessage = {
  [Type in WsEventType]: {
    type: Type;
    id?: string;
    timestamp?: string;
    data: z.infer<(typeof dataSchemas)[Type]>;
  };
}[WsEventType];

export function parseWsMessage(raw: string): Result<WSInboundMessage, string> {
  return fromThrowable(
    () => JSON.parse(raw) as unknown,
    () => 'Invalid WebSocket JSON',
  )().andThen((value) => {
    const envelope = messageEnvelopeSchema.safeParse(value);
    if (!envelope.success) return err('Invalid WebSocket envelope');
    if (!(envelope.data.type in dataSchemas))
      return err(`Unknown WebSocket event: ${envelope.data.type}`);
    const type = envelope.data.type as WsEventType;
    const payload = dataSchemas[type].safeParse(envelope.data.data);
    if (!payload.success) return err(`Invalid ${type} payload`);
    return ok({
      ...envelope.data,
      type,
      data: payload.data,
    } as WSInboundMessage);
  });
}

const ENGINE_PORT_POLL_INTERVAL_MS = 1000;

function isValidWsPort(port: unknown): port is number {
  return (
    typeof port === 'number' &&
    Number.isInteger(port) &&
    port > 0 &&
    port <= 65535
  );
}

/**
 * Resolves the sync-engine WebSocket URL synchronously when available.
 * Dynamic-port engines may create the renderer before readiness; in that state
 * this returns null so callers poll main rather than using legacy port 8080.
 */
function getWsUrl(): string | null {
  const electron = typeof window !== 'undefined' ? window.electron : undefined;
  if (electron === undefined) {
    return 'ws://127.0.0.1:8080/ws';
  }
  const port = isValidWsPort(resolvedWsPort) ? resolvedWsPort : electron.wsPort;
  return isValidWsPort(port) ? `ws://127.0.0.1:${port}/ws` : null;
}
const REQUEST_TIMEOUT_MS = 5000;
const PR_SYNC_RESPONSE_MARGIN_MS = 15000;
const PR_SYNC_REQUEST_TIMEOUT_MS = 2 * 60 * 1000 + PR_SYNC_RESPONSE_MARGIN_MS;
// CHECKOUT_PR_HEAD fetches and mutates HEAD; the generic 5s budget would
// report failure while the engine is still mid-checkout.
const CHECKOUT_REQUEST_TIMEOUT_MS = 60 * 1000;
const BASE_RECONNECT_DELAY_MS = 1000;
const MAX_RECONNECT_DELAY_MS = 30000;
const MAX_PORT_POLL_ATTEMPTS = 10;
const MAX_RECONNECT_BACKOFF_STEPS = 5;

// ── Singleton WebSocket manager ──────────────────────────────────────
let singletonWs: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let reconnectAttempt = 0;
let enginePortPollAttempt = 0;
let refCount = 0;
let resolvedWsPort: number | null = null;
let hasSentStartupDevFolderScan = false;
const messageHandlers = new Map<string, (data: unknown) => void>();

function handleSyncComplete(
  data: z.infer<typeof dataSchemas.SYNC_COMPLETE>,
): void {
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
}

function handlePRApproved(data: z.infer<typeof dataSchemas.PR_APPROVED>): void {
  appStore.setState((s) => ({
    ...s,
    lastSyncAt: data.timestamp,
  }));
}

function handleCommentPosted(): void {
  appStore.setState((s) => ({
    ...s,
    lastSyncAt: new Date().toISOString(),
    commentsRevision: s.commentsRevision + 1,
    threadsRevision: s.threadsRevision + 1,
  }));
}

function handleSyncError(data: z.infer<typeof dataSchemas.SYNC_ERROR>): void {
  appStore.setState((s) => ({
    ...s,
    syncStatus: 'error',
    syncError: `${data.error_code}: ${data.message}`,
  }));
}

function handleAuthExpired(
  data: z.infer<typeof dataSchemas.AUTH_TOKEN_EXPIRED>,
): void {
  appStore.setState((s) => ({
    ...s,
    syncStatus: 'error',
    syncError: `Auth expired (${data.provider}): ${data.message}`,
    isAuthenticated: false,
  }));
}

function handleRateLimited(
  data: z.infer<typeof dataSchemas.RATE_LIMITED>,
): void {
  appStore.setState((s) => ({
    ...s,
    syncStatus: 'rate_limited',
    syncError: `Rate limited until ${data.reset_at}`,
  }));
}

function handleActionComplete(
  data: z.infer<typeof dataSchemas.ACTION_COMPLETE>,
): void {
  appStore.setState((s) => ({
    ...s,
    pendingAction: null,
    actionResult: {
      type: data.action_type,
      prId: data.pr_id,
      success: true,
      message: data.message,
    },
    confirmingAction: null,
    lastSyncAt: new Date().toISOString(),
  }));
}

function handleActionFailed(
  data: z.infer<typeof dataSchemas.ACTION_FAILED>,
): void {
  appStore.setState((s) => ({
    ...s,
    pendingAction: null,
    actionResult: {
      type: data.action_type,
      prId: data.pr_id,
      success: false,
      message: data.error,
    },
    confirmingAction: null,
  }));
}

/**
 * A thread's resolved/reopened state changed on the server (our write landed,
 * or a teammate changed it). Bump `threadsRevision` so the open threads view
 * and merge-readiness badge re-fetch — no full sync cycle needed.
 */
function handleThreadStateChanged(
  _data: z.infer<typeof dataSchemas.THREAD_RESOLVED>,
): void {
  appStore.setState((s) => ({
    ...s,
    threadsRevision: s.threadsRevision + 1,
  }));
}

function bumpPRListRevision(): Result<void, unknown> {
  return fromThrowable(() => {
    appStore.setState((s) => ({
      ...s,
      prListRevision: s.prListRevision + 1,
    }));
  })();
}

function handlePRMetadataChanged(_data: unknown): void {
  bumpPRListRevision().mapErr((error) =>
    log.error({ err: error }, 'failed to invalidate PR list'),
  );
}

function handlePRsReadChanged(
  _data: z.infer<typeof dataSchemas.PRS_READ_CHANGED>,
): void {
  bumpPRListRevision().mapErr((error) =>
    log.error({ err: error }, 'failed to invalidate PR list'),
  );
}

function handleCIStatusChanged(_data: unknown): void {
  appStore.setState((s) => ({
    ...s,
    prListRevision: s.prListRevision + 1,
  }));
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

function handleExpertiseIndexProgress(
  data: z.infer<typeof dataSchemas.EXPERTISE_INDEX_PROGRESS>,
): void {
  mergeExpertiseStatus({
    repo_id: data.repo_id,
    status: 'indexing',
    files_done: data.files_done,
    files_total: data.files_total,
  });
}

function handleExpertiseReady(
  data: z.infer<typeof dataSchemas.EXPERTISE_READY>,
): void {
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

function handleExpertiseError(
  data: z.infer<typeof dataSchemas.EXPERTISE_ERROR>,
): void {
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

function handleExpertiseNudge(
  data: z.infer<typeof dataSchemas.EXPERTISE_NUDGE>,
): void {
  appStore.setState((s) => ({
    ...s,
    expertiseNudges: {
      ...s.expertiseNudges,
      [data.repo_id]: data,
    },
  }));
}

function handleExpertiseSettings(
  data: z.infer<typeof dataSchemas.EXPERTISE_SETTINGS>,
): void {
  appStore.setState((s) => ({
    ...s,
    expertiseSettings: data,
  }));
}

function handleScanDevFolderResult(
  data: unknown,
): z.infer<typeof scanDevFolderResultSchema> | null {
  const parsed = scanDevFolderResultSchema.safeParse(data);
  if (!parsed.success) {
    log.warn('invalid SCAN_DEV_FOLDER_RESULT payload');
    return null;
  }

  const payload = parsed.data;
  if (!('errors' in payload)) {
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
  const parsed = locationErrorSchema.safeParse(data);
  if (!parsed.success) {
    log.warn('invalid LOCATION_ERROR payload');
    return;
  }

  log.warn(
    {
      repoId: parsed.data.repo_id,
      operation: parsed.data.operation,
      code: parsed.data.code,
    },
    parsed.data.message,
  );
}

function handleAuthRedirectError(message: string): void {
  appStore.setState((state) => ({
    ...state,
    authLoading: false,
    authError: message,
  }));
  addToast({ type: 'error', message, dismissible: true });
}

function onMessage(event: MessageEvent<string>): void {
  const parsed = parseWsMessage(event.data);
  if (parsed.isErr()) {
    log.warn({ reason: parsed.error }, 'invalid WebSocket message');
    return;
  }
  const message = parsed.value;
  log.debug({ type: message.type, id: message.id }, 'received event');

  switch (message.type) {
    case 'SYNC_COMPLETE':
      handleSyncComplete(message.data);
      break;
    case 'SYNC_TRIGGERED': {
      if (message.data.queued === true) {
        appStore.setState((s) => ({ ...s, syncStatus: 'syncing' }));
      } else {
        log.warn('manual sync not queued — sync already in progress');
      }
      break;
    }
    case 'PR_APPROVED':
      handlePRApproved(message.data);
      break;
    case 'COMMENT_POSTED':
      handleCommentPosted();
      break;
    case 'SYNC_ERROR':
      handleSyncError(message.data);
      addToast({
        type: 'error',
        message: formatErrorMessage(message.data.error_code),
        dismissible: true,
      });
      break;
    case 'AUTH_TOKEN_EXPIRED':
      handleAuthExpired(message.data);
      addToast({
        type: 'error',
        message: formatErrorMessage('AUTH_TOKEN_EXPIRED'),
        dismissible: true,
      });
      break;
    case 'RATE_LIMITED':
      handleRateLimited(message.data);
      break;
    case 'ACTION_COMPLETE':
      handleActionComplete(message.data);
      break;
    case 'ACTION_FAILED':
      handleActionFailed(message.data);
      addToast({
        type: 'error',
        message: formatErrorMessage(message.data.error),
        dismissible: true,
      });
      break;
    case 'THREAD_RESOLVED':
    case 'THREAD_REOPENED':
      handleThreadStateChanged(message.data);
      break;
    case 'PR_METADATA_CHANGED':
      handlePRMetadataChanged(message.data);
      break;
    case 'PRS_READ_CHANGED':
      handlePRsReadChanged(message.data);
      break;
    case 'CI_STATUS_CHANGED':
      handleCIStatusChanged(message.data);
      break;
    case 'SCAN_DEV_FOLDER_RESULT': {
      const payload = handleScanDevFolderResult(message.data);
      if (payload === null) {
        break;
      }
      const scanHandler =
        message.id === undefined ? undefined : messageHandlers.get(message.id);
      if (scanHandler) {
        scanHandler(payload);
        if (message.id !== undefined) messageHandlers.delete(message.id);
      }
      break;
    }
    case 'LOCATION_ERROR':
      handleLocationError(message.data);
      break;
    case 'EXPERTISE_INDEX_PROGRESS':
      handleExpertiseIndexProgress(message.data);
      break;
    case 'EXPERTISE_READY':
      handleExpertiseReady(message.data);
      break;
    case 'EXPERTISE_ERROR': {
      const payload = message.data;
      handleExpertiseError(payload);
      const handler =
        message.id === undefined ? undefined : messageHandlers.get(message.id);
      if (handler) {
        handler({ error: payload.message || payload.code });
        if (message.id !== undefined) messageHandlers.delete(message.id);
      }
      break;
    }
    case 'EXPERTISE_NUDGE':
      handleExpertiseNudge(message.data);
      break;
    case 'EXPERTISE_DATA':
    case 'EXPERTISE_ENABLE_ACCEPTED':
    case 'EXPERTISE_DISABLE_ACCEPTED':
    case 'EXPERTISE_NUDGE_DISMISSED': {
      const payload = message.data;
      const handler =
        message.id === undefined ? undefined : messageHandlers.get(message.id);
      if (handler) {
        handler(payload);
        if (message.id !== undefined) messageHandlers.delete(message.id);
      }
      break;
    }
    case 'EXPERTISE_SETTINGS':
    case 'EXPERTISE_SETTINGS_UPDATED': {
      const payload = message.data;
      handleExpertiseSettings(payload);
      const handler =
        message.id === undefined ? undefined : messageHandlers.get(message.id);
      if (handler) {
        handler(payload);
        if (message.id !== undefined) messageHandlers.delete(message.id);
      }
      break;
    }
    case 'AUTH_REDIRECT': {
      const openExternal = window.electron?.openExternal;
      if (!openExternal) {
        appStore.setState((state) => ({
          ...state,
          authLoading: false,
          authError: 'Browser launch is unavailable.',
        }));
        break;
      }
      void ResultAsync.fromPromise(openExternal(message.data.url), (cause) =>
        cause instanceof Error ? cause.message : String(cause),
      ).match((result) => {
        if (result && 'err' in result) {
          handleAuthRedirectError(result.err);
          return;
        }
        appStore.setState((state) => ({
          ...state,
          authLoading: true,
          authError: null,
        }));
      }, handleAuthRedirectError);
      break;
    }
    case 'AUTH_CHECK_RESULT': {
      const payload = message.data;
      appStore.setState((s) => ({
        ...s,
        authChecked: true,
        authLoading: false,
        isAuthenticated: payload.authenticated,
        connectedProviders: payload.providers ?? [],
      }));
      break;
    }
    case 'AUTH_DISCONNECT_RESULT': {
      const payload = message.data;
      appStore.setState((s) => ({
        ...s,
        isAuthenticated: payload.authenticated,
        connectedProviders: payload.providers ?? [],
      }));
      break;
    }
    case 'AUTH_SUCCESS': {
      const payload = message.data;
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
      break;
    }
    case 'AUTH_FAILED': {
      const payload = message.data;
      appStore.setState((s) => ({
        ...s,
        authLoading: false,
        authError: payload.error,
      }));
      addToast({
        type: 'error',
        message: payload.error,
        dismissible: true,
      });
      break;
    }
    case 'COMMENTS_READY': {
      const handler =
        message.id === undefined ? undefined : messageHandlers.get(message.id);
      if (handler) {
        handler(message.data);
        if (message.id !== undefined) messageHandlers.delete(message.id);
        break;
      }
      // Server-initiated push (async LLM tier upgraded classifications):
      // no matching request id — bump commentsRevision so open views refetch.
      appStore.setState((s) => ({
        ...s,
        commentsRevision: s.commentsRevision + 1,
      }));
      break;
    }
    case 'BYOK_USAGE': {
      const payload = message.data;
      appStore.setState((s) => ({
        ...s,
        byokSessionTokens: payload.session_tokens ?? 0,
        byokEstCost: payload.est_cost ?? 0,
      }));
      break;
    }
    case 'BYOK_AUTH_FAILED': {
      appStore.setState((s) => ({ ...s, byokAuthFailed: true }));
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
    case 'PR_CHECKOUT_STATUS':
    case 'PR_CHECKOUT_RESULT':
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
      const handler =
        message.id === undefined ? undefined : messageHandlers.get(message.id);
      if (handler) {
        handler(message.data);
        if (message.id !== undefined) messageHandlers.delete(message.id);
      }
      break;
    }
    case 'REPO_LIST_READY': {
      const payload = message.data;
      if (payload.error) {
        appStore.setState((state) => ({ ...state, reposLoading: false }));
        addToast({
          type: 'error',
          message: `Failed to load repos: ${payload.error}`,
          dismissible: true,
        });
      } else {
        appStore.setState((state) => ({
          ...state,
          repos: (payload.repos ?? []).map(
            ({
              source,
              location_status,
              location_error_code,
              location_error,
              ...repo
            }) => ({
              ...repo,
              ...(source === undefined ? {} : { source }),
              ...(location_status === undefined ? {} : { location_status }),
              ...(location_error_code === undefined
                ? {}
                : { location_error_code }),
              ...(location_error === undefined ? {} : { location_error }),
            }),
          ),
          reposLoading: false,
        }));
      }
      const repoListHandler =
        message.id === undefined ? undefined : messageHandlers.get(message.id);
      if (repoListHandler) {
        repoListHandler(message.data);
        if (message.id !== undefined) messageHandlers.delete(message.id);
      }
      break;
    }
    case 'WATCH_REPO_RESULT': {
      const payload = message.data;
      appStore.setState((s) => ({
        ...s,
        repos: s.repos.map((r) =>
          r.id === payload.repo_id ? { ...r, watched: payload.watched } : r,
        ),
      }));
      const watchHandler =
        message.id === undefined ? undefined : messageHandlers.get(message.id);
      if (watchHandler) {
        watchHandler(message.data);
        if (message.id !== undefined) messageHandlers.delete(message.id);
      }
      break;
    }
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
  fromThrowable(() => ws.send(JSON.stringify(scanMsg)))().mapErr((error) =>
    log.warn({ err: error }, 'startup scan request failed'),
  );
}

function connect(): void {
  reconnectTimer = null;
  if (
    refCount === 0 ||
    (singletonWs && singletonWs.readyState <= WebSocket.OPEN)
  ) {
    return;
  }

  const wsUrl = getWsUrl();
  if (wsUrl === null) {
    if (enginePortPollAttempt >= MAX_PORT_POLL_ATTEMPTS) {
      log.error(
        { maxAttempts: MAX_PORT_POLL_ATTEMPTS },
        'sync engine WebSocket port was not published',
      );
      appStore.setState((state) => ({
        ...state,
        syncStatus: 'error',
        syncError: 'Sync engine did not start. Please restart the app.',
        authChecked: true,
        isAuthenticated: false,
      }));
      return;
    }

    enginePortPollAttempt += 1;
    const getWsPort = window.electron?.getWsPort;
    if (getWsPort !== undefined) {
      void ResultAsync.fromPromise(getWsPort(), (error) => error).match(
        (port) => {
          if (isValidWsPort(port)) resolvedWsPort = port;
        },
        (error) => {
          log.warn(
            { err: error, attempt: enginePortPollAttempt },
            'failed to query sync engine WebSocket port',
          );
        },
      );
    }
    log.info(
      { attempt: enginePortPollAttempt, maxAttempts: MAX_PORT_POLL_ATTEMPTS },
      'Go sync engine is still starting — waiting for its WebSocket port',
    );
    reconnectTimer = setTimeout(connect, ENGINE_PORT_POLL_INTERVAL_MS);
    return;
  }

  enginePortPollAttempt = 0;

  log.info({ url: wsUrl }, 'attempting connection');
  appStore.setState((s) => ({ ...s, syncStatus: 'connecting' }));
  const socket = fromThrowable(() => new WebSocket(wsUrl))();
  if (socket.isErr()) {
    log.error({ err: socket.error }, 'failed to create WebSocket');
    appStore.setState((state) => ({
      ...state,
      syncStatus: 'error',
      syncError: 'WebSocket connection failed',
    }));
    reconnectTimer = setTimeout(connect, MAX_RECONNECT_DELAY_MS);
    return;
  }
  const ws = socket.value;

  ws.onopen = () => {
    log.info({ url: wsUrl }, 'connected');
    reconnectAttempt = 0;
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'connected',
      syncError: null,
      lastSyncAt: new Date().toISOString(),
      isOffline: false,
      authChecked: false,
    }));

    const checkMsg: WSMessage = {
      id: `req-${Date.now()}-${Math.random()}`,
      type: 'AUTH_CHECK',
      data: {},
      timestamp: new Date().toISOString(),
    };
    log.info({ id: checkMsg.id }, 'sending AUTH_CHECK');
    fromThrowable(() => ws.send(JSON.stringify(checkMsg)))().mapErr((error) =>
      log.warn({ err: error }, 'auth check request failed'),
    );
    sendStartupDevFolderScan(ws);
  };

  ws.onmessage = onMessage;

  ws.onerror = (error) => {
    log.error({ err: error }, 'connection error');
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'error',
      syncError: 'WebSocket connection failed',
    }));
  };

  ws.onclose = () => {
    if (singletonWs !== ws) return;
    log.info('disconnected');
    singletonWs = null;
    // Each new connection should re-run the startup dev-folder scan;
    // disconnect() resets this too, but dropped sockets reconnect directly.
    hasSentStartupDevFolderScan = false;
    const isOnline = typeof navigator !== 'undefined' ? navigator.onLine : true;
    appStore.setState((s) => ({
      ...s,
      syncStatus: 'disconnected',
      isOffline: !isOnline,
      authLoading: false,
    }));

    if (refCount > 0) {
      const delay = Math.min(
        BASE_RECONNECT_DELAY_MS * 2 ** reconnectAttempt,
        MAX_RECONNECT_DELAY_MS,
      );
      reconnectAttempt = Math.min(
        reconnectAttempt + 1,
        MAX_RECONNECT_BACKOFF_STEPS,
      );
      log.info({ delay }, 'scheduling reconnect');
      reconnectTimer = setTimeout(connect, delay);
    }
  };

  singletonWs = ws;
}

function disconnect(): void {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  const socket = singletonWs;
  singletonWs = null;
  if (socket) {
    socket.onopen = null;
    socket.onmessage = null;
    socket.onerror = null;
    socket.onclose = null;
    socket.close();
  }
  reconnectAttempt = 0;
  enginePortPollAttempt = 0;
  resolvedWsPort = null;
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
    log.warn({ type }, 'dropped message');
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
  const sent = fromThrowable(() => ws.send(JSON.stringify(message)))();
  if (sent.isErr()) {
    log.error(
      { type, id, err: sent.error },
      'failed to send WebSocket message',
    );
    return undefined;
  }
  return id;
}

/**
 * Extracts a non-empty `error` string from a response payload, or null.
 * Go handlers report recoverable failures (e.g. "diff not synced yet") as
 * an `error` field inside otherwise-successful *_READY envelopes.
 */
const responseErrorSchema = z.object({ error: z.unknown().optional() });

export function extractResponseError(data: unknown): string | null {
  const parsed = responseErrorSchema.safeParse(data);
  if (!parsed.success) return null;
  const error = parsed.data.error;
  return typeof error === 'string' && error.length > 0 ? error : null;
}

const CONNECT_WAIT_INTERVAL_MS = 250;

function requestTimeoutFor(type: string): number {
  switch (type) {
    case 'REQUEST_PR_SYNC':
      return PR_SYNC_REQUEST_TIMEOUT_MS;
    case 'CHECKOUT_PR_HEAD':
      return CHECKOUT_REQUEST_TIMEOUT_MS;
    default:
      return REQUEST_TIMEOUT_MS;
  }
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
