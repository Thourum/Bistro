import { Fault, fromThrowable, type Result } from '@thourum/fault';

const BENCH_QUERY_PREFIX = '--bench-query=';
const QUIT_AFTER_READY_ARG = '--quit-after-ready';

/** Builds renderer search parameters from the benchmark launch arguments. */
export function parseBenchSearch(
  argv: readonly string[],
): Result<string, Fault> {
  return fromThrowable(
    () => {
      const queries = argv
        .filter((arg) => arg.startsWith(BENCH_QUERY_PREFIX))
        .map((arg) => decodeURIComponent(arg.slice(BENCH_QUERY_PREFIX.length)));
      const quitSearch = argv.includes(QUIT_AFTER_READY_ARG)
        ? ['bench_quit_after_ready=1']
        : [];
      return [...quitSearch, ...queries].filter(Boolean).join('&');
    },
    (cause) =>
      new Fault('Invalid --bench-query encoding')
        .withTag('CONFIGURATION_ERROR')
        .withCause(cause),
  )();
}
