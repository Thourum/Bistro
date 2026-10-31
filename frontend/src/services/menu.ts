import { Shell } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { call } from './engine/call';

/** Native UI state uses call so availability also stays current in key benches. */
export function reportMenuActions(
  enabled: readonly string[],
): ResultAsync<void, Fault> {
  return call(() => Shell.SetMenuActions({ enabled: [...enabled] }));
}
