import { Shell } from '@bindings/services';
import { afterEach, expect, it, vi } from 'vitest';
import { cancellable } from '../test/cancellable';
import { openUrlInBrowser } from './openUrl';

vi.mock('@bindings/services', () => ({ Shell: { OpenExternal: vi.fn() } }));
afterEach(() => vi.clearAllMocks());

it('returns a shell opening failure to the UI boundary', async () => {
  vi.mocked(Shell.OpenExternal).mockReturnValue(
    cancellable(Promise.reject(new Error('browser unavailable'))),
  );
  const result = await openUrlInBrowser('https://example.com/pr');
  expect(result._unsafeUnwrapErr().message).toBe('browser unavailable');
});
