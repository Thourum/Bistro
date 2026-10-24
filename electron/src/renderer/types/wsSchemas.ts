import {
  err,
  type Fault,
  fromThrowable,
  ok,
  type Result,
  ServiceError,
} from '@thourum/fault';
import { z } from 'zod';

const messageEnvelopeSchema = z.object({
  id: z.string().optional(),
  type: z.string(),
  data: z.unknown().optional(),
  timestamp: z.string().optional(),
});

export type WSMessage = z.infer<typeof messageEnvelopeSchema>;

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
  // The async scan worker reports failures on the same correlated result.
  z.object({ ok: z.literal(false), code: z.string(), message: z.string() }),
]);

export const dataSchemas = Object.freeze({
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
    exchange: z.enum(['proxy', 'local']).optional(),
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
} satisfies Record<string, z.ZodType>);

export type WsEventType = keyof typeof dataSchemas;
export type WsEventData<Type extends WsEventType> = z.infer<
  (typeof dataSchemas)[Type]
>;

function eventSchema<Type extends WsEventType>(type: Type) {
  return messageEnvelopeSchema.extend({
    type: z.literal(type),
    data: dataSchemas[type],
  });
}

const inboundMessageSchema = z.discriminatedUnion('type', [
  eventSchema('SYNC_COMPLETE'),
  eventSchema('SYNC_TRIGGERED'),
  eventSchema('PR_APPROVED'),
  eventSchema('COMMENT_POSTED'),
  eventSchema('SYNC_ERROR'),
  eventSchema('AUTH_TOKEN_EXPIRED'),
  eventSchema('RATE_LIMITED'),
  eventSchema('ACTION_COMPLETE'),
  eventSchema('ACTION_FAILED'),
  eventSchema('THREAD_RESOLVED'),
  eventSchema('THREAD_REOPENED'),
  eventSchema('PR_METADATA_CHANGED'),
  eventSchema('PRS_READ_CHANGED'),
  eventSchema('CI_STATUS_CHANGED'),
  eventSchema('SCAN_DEV_FOLDER_RESULT'),
  eventSchema('LOCATION_ERROR'),
  eventSchema('EXPERTISE_INDEX_PROGRESS'),
  eventSchema('EXPERTISE_READY'),
  eventSchema('EXPERTISE_ERROR'),
  eventSchema('EXPERTISE_NUDGE'),
  eventSchema('EXPERTISE_DATA'),
  eventSchema('EXPERTISE_ENABLE_ACCEPTED'),
  eventSchema('EXPERTISE_DISABLE_ACCEPTED'),
  eventSchema('EXPERTISE_NUDGE_DISMISSED'),
  eventSchema('EXPERTISE_SETTINGS'),
  eventSchema('EXPERTISE_SETTINGS_UPDATED'),
  eventSchema('AUTH_REDIRECT'),
  eventSchema('AUTH_CHECK_RESULT'),
  eventSchema('AUTH_DISCONNECT_RESULT'),
  eventSchema('AUTH_SUCCESS'),
  eventSchema('AUTH_FAILED'),
  eventSchema('COMMENTS_READY'),
  eventSchema('BYOK_USAGE'),
  eventSchema('BYOK_AUTH_FAILED'),
  eventSchema('PR_SYNCED'),
  eventSchema('PRS_READ_ACK'),
  eventSchema('DIFF_READY'),
  eventSchema('PATCHSETS_READY'),
  eventSchema('RANGE_DIFF_READY'),
  eventSchema('FILE_VIEWED_ACK'),
  eventSchema('REVIEW_THREADS_READY'),
  eventSchema('ACTION_RESULT'),
  eventSchema('PR_CHECKOUT_STATUS'),
  eventSchema('PR_CHECKOUT_RESULT'),
  eventSchema('VALIDATE_DEV_FOLDER_RESULT'),
  eventSchema('ADD_REPO_BY_URL_RESULT'),
  eventSchema('GET_CACHE_STATS_RESULT'),
  eventSchema('SET_EXPERTISE_SETTINGS'),
  eventSchema('PRUNE_CACHE_RESULT'),
  eventSchema('CLEAR_CACHE_RESULT'),
  eventSchema('DELETE_CACHE_REPO_RESULT'),
  eventSchema('RECOVER_REPO_PATH_RESULT'),
  eventSchema('RECOVER_REPO_TO_CACHE_RESULT'),
  eventSchema('REMOVE_REPO_PREFLIGHT_RESULT'),
  eventSchema('REMOVE_REPO_RESULT'),
  eventSchema('CLASSIFICATION_SETTINGS_UPDATED'),
  eventSchema('BYOK_KEY_UPDATED'),
  eventSchema('DISMISS_RESULT'),
  eventSchema('UNDISMISS_RESULT'),
  eventSchema('PR_LIST_READY'),
  eventSchema('REPO_LIST_READY'),
  eventSchema('WATCH_REPO_RESULT'),
]);

// The default must satisfy the parsed union, so omitted event schemas fail tsc.
export type WSInboundMessage<
  Type extends z.infer<typeof inboundMessageSchema>['type'] = WsEventType,
> = {
  [Event in Type]: {
    type: Event;
    data: WsEventData<Event>;
  } & Pick<WSMessage, 'id' | 'timestamp'>;
}[Type];

/** Parses and validates an inbound event before domain handlers see its data. */
export function parseWsMessage(raw: string): Result<WSInboundMessage, Fault> {
  return fromThrowable(
    (): unknown => JSON.parse(raw),
    () => ServiceError('PARSE_ERROR', 'Invalid WebSocket JSON'),
  )().andThen((value) => {
    const envelope = messageEnvelopeSchema.safeParse(value);
    if (!envelope.success) {
      return err(
        ServiceError('VALIDATION_ERROR', 'Invalid WebSocket envelope'),
      );
    }
    if (!Object.hasOwn(dataSchemas, envelope.data.type)) {
      return err(
        ServiceError(
          'BAD_REQUEST',
          `Unknown WebSocket event: ${envelope.data.type}`,
        ),
      );
    }
    const message = inboundMessageSchema.safeParse({
      ...envelope.data,
      data: envelope.data.data,
    });
    if (!message.success) {
      return err(
        ServiceError(
          'VALIDATION_ERROR',
          `Invalid ${envelope.data.type} payload`,
        ),
      );
    }
    return ok(message.data);
  });
}

const responseErrorSchema = z.object({ error: z.unknown().optional() });

/**
 * Extracts a non-empty error string from an otherwise-successful response.
 * Go handlers report recoverable failures inside *_READY envelopes.
 */
export function extractResponseError(data: unknown): string | null {
  const parsed = responseErrorSchema.safeParse(data);
  if (!parsed.success) return null;
  const error = parsed.data.error;
  return typeof error === 'string' && error.length > 0 ? error : null;
}
