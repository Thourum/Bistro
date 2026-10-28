import {
  err,
  Fault,
  fromThrowable,
  ok,
  type Result,
  ServiceError,
} from '@thourum/fault';
import { z } from 'zod';
import { benchCollectorSchema } from '../shared/benchSafety';

const keysFixtureSchema = z.enum([
  'github:bench/large-diff:1',
  'github:bench/large-diff:2',
]);

/** Validates keyboard benchmark settings before any app work can start. */
export function validateKeysBenchSearch(search: string): Result<void, Fault> {
  const params = new URLSearchParams(search);
  if (!params.has('bench_keys')) return ok(undefined);
  const collector = benchCollectorSchema.safeParse(
    params.get('bench_collector'),
  );
  if (
    params.getAll('bench_keys').length !== 1 ||
    params.get('bench_keys') !== '1' ||
    !collector.success ||
    !keysFixtureSchema.safeParse(params.get('bench_keys_pr')).success
  ) {
    return err(
      ServiceError(
        'CONFIGURATION_ERROR',
        'Keyboard benchmark requires bench_keys=1, a loopback HTTP collector port and a seeded synthetic fixture',
      ),
    );
  }
  return ok(undefined);
}

/** Reads the validated native keyboard benchmark flag. */
export function isKeysBenchSearch(search: string): boolean {
  return new URLSearchParams(search).get('bench_keys') === '1';
}

const BENCH_QUERY_PREFIX = '--bench-query=';
const QUIT_AFTER_READY_ARG = '--quit-after-ready';

/** Builds renderer search parameters from the benchmark launch arguments. */
export function parseBenchSearch(
  argv: readonly string[],
  initialSearch = '',
): Result<string, Fault> {
  return fromThrowable(
    () => {
      const queries = argv
        .filter((arg) => arg.startsWith(BENCH_QUERY_PREFIX))
        .map((arg) => decodeURIComponent(arg.slice(BENCH_QUERY_PREFIX.length)));
      const quitSearch = argv.includes(QUIT_AFTER_READY_ARG)
        ? ['bench_quit_after_ready=1']
        : [];
      return [initialSearch.replace(/^\?/, ''), ...quitSearch, ...queries]
        .filter(Boolean)
        .join('&');
    },
    (cause) =>
      new Fault('Invalid --bench-query encoding')
        .withTag('CONFIGURATION_ERROR')
        .withCause(cause),
  )().andThen((search) => validateKeysBenchSearch(search).map(() => search));
}
