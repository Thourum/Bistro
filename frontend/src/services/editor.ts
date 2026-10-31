import {
  type CheckoutResult,
  type CheckoutStatus,
  Editor,
  Shell,
} from '@bindings/services';
import type { OpenInEditorRequest } from '@bindings/shell/models';
import type { Fault, ResultAsync } from '@thourum/fault';
import { call, callWrite } from './engine/call';

const CHECKOUT_TIMEOUT_MS = 60_000;

export function checkPRCheckout(
  prId: string,
): ResultAsync<CheckoutStatus, Fault> {
  return call(() => Editor.CheckoutStatus({ pr_id: prId }));
}

export function checkoutPRHead(
  prId: string,
  expectedHeadSHA: string,
): ResultAsync<CheckoutResult, Fault> {
  return callWrite(
    () =>
      Editor.CheckoutHead({ pr_id: prId, expected_head_sha: expectedHeadSHA }),
    { timeoutMs: CHECKOUT_TIMEOUT_MS },
  );
}

export function openInEditor(
  request: Readonly<OpenInEditorRequest>,
): ResultAsync<void, Fault> {
  return callWrite(() => Shell.OpenInEditor(request));
}
