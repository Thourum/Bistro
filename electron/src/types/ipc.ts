/**
 * IPC Message Envelope
 *
 * All IPC messages follow this pattern:
 * - Request: { ok?: never; err?: never; method: string; params?: unknown }
 * - Response: { ok: T } | { err: string }
 */

export interface IPCError {
  err: string;
}

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

// ---- Open in Editor Channel ----

export interface OpenInEditorRequest {
  /** Local repo root (from CHECK_PR_CHECKOUT's local_path). */
  repoRoot: string;
  /** Repo-relative file path; main resolves+validates against repoRoot. */
  relativePath: string;
  line: number;
}

export interface OpenInEditorResponse {
  ok: true;
}

// ---- Go Command Channel ----

// Review-thread resolve/unresolve actions intentionally do not use this legacy
// IPC command surface. They go over the sync engine WebSocket as
// SUBMIT_ACTION payloads (`resolve_thread` / `unresolve_thread`) so they share
// the same offline queue, retry, and ACTION_RESULT semantics as other PR writes.

export interface GoCommandRequest {
  type: 'approve_pr' | 'request_changes' | 'post_comment';
  pr_id: string;
  comment?: string;
  line?: number;
  file?: string;
}

export interface GoCommandResponse {
  ok: boolean;
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
