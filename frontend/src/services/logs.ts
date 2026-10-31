import { Shell } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { callWrite } from './engine/call';
import { addToast } from './toasts';

/** Opens the log folder and displays any failure. */
export function openLogFolder(): ResultAsync<void, Fault> {
  return callWrite(() => Shell.OpenLogFolder()).orInspect((fault) =>
    addToast({ type: 'error', message: fault.message, dismissible: true }),
  );
}
