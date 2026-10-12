/**
 * IPC Message Envelope
 *
 * All IPC messages follow this pattern:
 * - Request: { ok?: never; err?: never; method: string; params?: unknown }
 * - Response: { ok: T } | { err: string }
 */

import { z } from 'zod';

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

export const fsReadPatchRequestSchema = z.object({ path: z.string().min(1) });
export type FSReadPatchRequest = z.infer<typeof fsReadPatchRequestSchema>;

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

export const goCommandRequestSchema = z.object({
  type: z.enum(['approve_pr', 'request_changes', 'post_comment']),
  pr_id: z.string().min(1),
  comment: z.string().optional(),
  line: z.number().int().optional(),
  file: z.string().optional(),
});
export type GoCommandRequest = z.infer<typeof goCommandRequestSchema>;

export interface GoCommandResponse {
  ok: boolean;
}

// ---- Sync Action Channel ----

export const syncActionRequestSchema = goCommandRequestSchema.pick({
  type: true,
  pr_id: true,
  comment: true,
  line: true,
});
export type SyncActionRequest = z.infer<typeof syncActionRequestSchema>;

export interface SyncActionResponse {
  ok: { success: boolean; message?: string };
}
