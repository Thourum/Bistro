import { expect, it } from 'vitest';
import { parseBenchSearch } from './benchArgs';

it.each([
  'bench_keys=1',
  'bench_keys=0&bench_collector=http://localhost:9000&bench_keys_pr=github:bench/large-diff:1',
  'bench_keys=1&bench_collector=file:///tmp/report&bench_keys_pr=github:bench/large-diff:1',
  'bench_keys=1&bench_collector=http://localhost:9000&bench_keys_pr=github:real/repo:1',
])('fails closed for incomplete or invalid keyboard benchmark: %s', (query) => {
  expect(
    parseBenchSearch([
      `--bench-query=${encodeURIComponent(query)}`,
    ])._unsafeUnwrapErr().tag,
  ).toBe('CONFIGURATION_ERROR');
});

it.each([
  'https://localhost:9000',
  'http://example.com:9000',
  'http://127.0.0.1',
  'http://127.0.0.1:0',
  'http://127.0.0.1:65536',
  'http://127.0.0.1:9000/path',
  'http://user@localhost:9000',
])('rejects non-loopback collector %s', (collector) => {
  const query = `bench_keys=1&bench_keys_pr=github:bench/large-diff:1&bench_collector=${encodeURIComponent(collector)}`;
  expect(parseBenchSearch([], query).isErr()).toBe(true);
});

it('validates dev URL keyboard flags as well as argv', () => {
  const query =
    'bench_keys=1&bench_collector=http://localhost:9000&bench_keys_pr=github:bench/large-diff:1';
  expect(parseBenchSearch([], query)._unsafeUnwrap()).toBe(query);
  expect(parseBenchSearch([], 'bench_keys=1').isErr()).toBe(true);
});

it('decodes the bench query once and adds the ready-quit flag without consuming unrelated argv', () => {
  const query =
    'bench_collector=http://127.0.0.1:9000&bench_diff=github%3Abench%2Flarge-diff%3A1';
  const parsed = parseBenchSearch([
    '/Bistro',
    '--user-data-dir=/tmp/bench',
    '--quit-after-ready',
    `--bench-query=${encodeURIComponent(query)}`,
  ]);
  expect(parsed.isOk()).toBe(true);
  if (parsed.isErr()) return;
  expect(parsed.value).toBe(`bench_quit_after_ready=1&${query}`);
  expect(new URLSearchParams(parsed.value).get('bench_diff')).toBe(
    'github:bench/large-diff:1',
  );
  expect(parseBenchSearch(['/Bistro'])._unsafeUnwrap()).toBe('');
  expect(parseBenchSearch(['--bench-query=%ZZ']).isErr()).toBe(true);
});
