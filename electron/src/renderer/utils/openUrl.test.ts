// @vitest-environment jsdom

import { afterEach, expect, it, vi } from 'vitest';
import { openUrlInBrowser } from './openUrl';

afterEach(() => vi.restoreAllMocks());

it('treats a noopener browser tab as opened even when window.open returns null', async () => {
  const open = vi.spyOn(window, 'open').mockReturnValue(null);

  const result = await openUrlInBrowser('https://example.com/pr');

  expect(open).toHaveBeenCalledWith(
    'https://example.com/pr',
    '_blank',
    'noopener,noreferrer',
  );
  expect(result.isOk()).toBe(true);
});
