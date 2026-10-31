import { err, type Fault, ok, type Result, ServiceError } from '@thourum/fault';
import { emitAppEvent } from './appEvents';

// Latch before user input: history/query changes cannot disable the safety guard.
const IS_KEYS_BENCH =
  typeof location !== 'undefined' &&
  new URLSearchParams(location.search).get('bench_keys') === '1';

/** True only for the opt-in native keyboard benchmark at renderer startup. */
export function isKeysBench(): boolean {
  return IS_KEYS_BENCH;
}

/** Records intended action dispatch without executing any benchmark handler. */
export function permitsBenchAction(actionId: string): boolean {
  if (!isKeysBench()) return true;
  emitAppEvent('bistro:bench:action', { actionId });
  return false;
}

/** Refuses a side effect before transport/IPC, and reports it to the collector. */
export function guardBenchWrite(type: string): Result<void, Fault> {
  if (!isKeysBench()) return ok(undefined);
  emitAppEvent('bistro:bench:blocked-write', { type });
  return err(
    ServiceError('BENCH_WRITE_BLOCKED', `Keys benchmark blocked ${type}`),
  );
}
