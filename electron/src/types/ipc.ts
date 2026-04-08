/**
 * IPC Message Envelope
 *
 * All IPC messages follow this pattern:
 * - Request: { ok?: never; err?: never; method: string; params?: unknown }
 * - Response: { ok: T } | { err: string }
 */

export interface IPCRequest {
  method: string;
  params?: unknown;
}

export interface IPCSuccess<T> {
  ok: T;
}

export interface IPCError {
  err: string;
}

export type IPCResponse<T> = IPCSuccess<T> | IPCError;

// ---- DB Query Channel ----

export interface DBQueryRequest {
  sql: string;
  params?: unknown[];
}

export interface DBQueryResponse {
  ok: Record<string, unknown>[];
}

// ---- File Read Channel ----

export interface FSReadPatchRequest {
  path: string;
}

export interface FSReadPatchResponse {
  ok: string;
}

// ---- Go Command Channel ----

export interface GoCommandRequest {
  type: 'approve_pr' | 'request_changes' | 'post_comment';
  pr_id: string;
  comment?: string;
  line?: number;
}

export interface GoCommandResponse {
  ok: boolean;
}

// ---- Sync Subscribe Channel ----

export interface SyncSubscribeRequest {
  // No params — subscribes renderer to all sync events
}

// ---- Sync Action Channel ----

export interface SyncActionRequest {
  type: 'approve_pr' | 'request_changes' | 'post_comment';
  pr_id: string;
  comment?: string;
  line?: number;
}

export interface SyncActionResponse {
  ok: { success: boolean; message?: string };
}
