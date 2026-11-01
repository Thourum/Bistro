import { Shell } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { callWrite } from '../services/engine/call';

/** Opens a URL in the user's browser through the desktop shell. */
export function openUrlInBrowser(url: string): ResultAsync<void, Fault> {
  return callWrite(() => Shell.OpenExternal(url));
}
