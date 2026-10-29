import { describe, expect, it } from 'vitest';
import { firstHunkAdditionLine } from './firstHunkLine';

describe('firstHunkAdditionLine', () => {
  it('parses the +line from the first hunk header', () => {
    const patch = ['@@ -10,3 +12,4 @@', ' context', '+added', ' context'].join(
      '\n',
    );
    expect(firstHunkAdditionLine(patch)).toBe(12);
  });

  it('returns 1 when the patch has no hunks', () => {
    expect(firstHunkAdditionLine('diff --git a/foo b/foo\n')).toBe(1);
  });
});
