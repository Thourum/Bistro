import type { OpenInEditorRequest } from '@renderer/types/electron/ipc';
import { err, Fault, fromPromise, ok, ResultAsync } from '@thourum/fault';
import { safeZodParse } from '@thourum/fault/zod';
import { z } from 'zod';
import { guardBenchWrite } from '../utils/benchSafety';
import { parseResponse } from '../utils/error';
import { wsClient } from './ws/connection';

const checkPRCheckoutSchema = z.object({
  local_path: z.string().optional(),
  head_sha: z.string().optional(),
  head_matches: z.boolean().optional(),
  error: z.string().optional(),
});

const checkoutPRHeadSchema = z.object({
  ok: z.boolean().optional(),
  local_path: z.string().optional(),
  reason: z.string().optional(),
});

const openInEditorResponseSchema = z.union([
  z.object({ ok: z.literal(true) }),
  z.object({ err: z.string() }),
]);

type PRCheckout = z.infer<typeof checkPRCheckoutSchema>;
type CheckoutPRHeadResponse = z.infer<typeof checkoutPRHeadSchema>;

/** Checks the local checkout against the PR head before offering editor actions. */
export function checkPRCheckout(prId: string): ResultAsync<PRCheckout, Fault> {
  return wsClient
    .request('CHECK_PR_CHECKOUT', { pr_id: prId })
    .mapErr((cause) =>
      new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
    )
    .andThen((data) => parseResponse(checkPRCheckoutSchema, data));
}

/** Checks out the inspected head; the engine rejects a changed PR head SHA. */
export function checkoutPRHead(
  prId: string,
  expectedHeadSha: string,
): ResultAsync<CheckoutPRHeadResponse, Fault> {
  return wsClient
    .request('CHECKOUT_PR_HEAD', {
      pr_id: prId,
      expected_head_sha: expectedHeadSha,
    })
    .mapErr((cause) =>
      new Fault(cause).withTag('NETWORK_ERROR').withCause(cause),
    )
    .andThen((data) => parseResponse(checkoutPRHeadSchema, data));
}

/** Opens the working-tree file through the validated main-process IPC bridge. */
export function openInEditor(
  request: Readonly<OpenInEditorRequest>,
): ResultAsync<void, Fault> {
  const permitted = guardBenchWrite('open-in-editor');
  if (permitted.isErr()) return new ResultAsync(Promise.resolve(permitted));
  return fromPromise(window.electron.openInEditor(request), (cause) =>
    new Fault(cause instanceof Error ? cause : 'Unknown error')
      .withTag('EXTERNAL_ERROR')
      .withCause(cause),
  )
    .andThen((response) => safeZodParse(openInEditorResponseSchema, response))
    .andThen((response) =>
      'err' in response
        ? err(new Fault(response.err).withTag('EDITOR_ERROR'))
        : ok(undefined),
    );
}
