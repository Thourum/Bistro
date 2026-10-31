import { Shell } from '@bindings/services';
import type { Fault, ResultAsync } from '@thourum/fault';
import { call } from './engine/call';

/** Native UI state uses call so availability also stays current in key benches. */
export function reportMenuActions(
  enabled: readonly string[],
): ResultAsync<void, Fault> {
  return call(() => Shell.SetMenuActions({ enabled: [...enabled] }));
}

interface MenuActionReporterOptions {
  readonly send: (enabled: readonly string[]) => ResultAsync<void, Fault>;
  readonly onError: (fault: Fault) => void;
}

/** Serializes native snapshots, retaining only the latest while a send settles. */
export function createMenuActionReporter({
  send,
  onError,
}: MenuActionReporterOptions): (enabled: readonly string[]) => void {
  let isSending = false;
  let pending: readonly string[] | null = null;

  function finish(): void {
    isSending = false;
    sendNext();
  }

  function sendNext(): void {
    if (isSending || pending === null) return;
    const enabled = pending;
    pending = null;
    isSending = true;
    void send(enabled).match(finish, (fault) => {
      onError(fault);
      finish();
    });
  }

  return (enabled) => {
    pending = [...enabled];
    sendNext();
  };
}
