import { expect, it } from 'vitest';
import { parseBenchSearch } from './benchArgs';

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
