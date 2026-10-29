import { describe, expect, it } from 'vitest';
import {
  hideWhitespaceOnlyHunks,
  isWhitespaceOnlyHunk,
} from './whitespaceDiff';

function hunk(...lines: string[]): string[] {
  return lines;
}

describe('isWhitespaceOnlyHunk', () => {
  it('returns true for an indentation-only change', () => {
    expect(
      isWhitespaceOnlyHunk(
        hunk('@@ -1,1 +1,1 @@', '-  const a = 1;', '+const a = 1;'),
      ),
    ).toBe(true);
  });

  it('returns true for a tab-to-space conversion', () => {
    expect(
      isWhitespaceOnlyHunk(
        hunk('@@ -1,1 +1,1 @@', '-\tif (a) {', '+  if (a) {'),
      ),
    ).toBe(true);
  });

  it('returns true when whitespace moves across multiple lines', () => {
    expect(
      isWhitespaceOnlyHunk(
        hunk(
          '@@ -1,2 +1,2 @@',
          '-  a();',
          '-  b();',
          '+a();',
          '+b();',
          ' unchanged',
        ),
      ),
    ).toBe(true);
  });

  it('returns false for a real code change', () => {
    expect(
      isWhitespaceOnlyHunk(
        hunk('@@ -1,1 +1,1 @@', '-const a = 1;', '+const a = 2;'),
      ),
    ).toBe(false);
  });

  it('returns false when a line is added with no counterpart', () => {
    expect(isWhitespaceOnlyHunk(hunk('@@ -1,0 +1,1 @@', '+const a = 1;'))).toBe(
      false,
    );
  });

  it('returns false when a line is removed with no counterpart', () => {
    expect(isWhitespaceOnlyHunk(hunk('@@ -1,1 +1,0 @@', '-const a = 1;'))).toBe(
      false,
    );
  });

  it('returns false for a context-only hunk', () => {
    expect(isWhitespaceOnlyHunk(hunk('@@ -1,1 +1,1 @@', ' unchanged'))).toBe(
      false,
    );
  });

  it('ignores the "\\ No newline at end of file" marker', () => {
    expect(
      isWhitespaceOnlyHunk(
        hunk('@@ -1,1 +1,1 @@', '-  a', '\\ No newline at end of file', '+a'),
      ),
    ).toBe(true);
  });
});

describe('hideWhitespaceOnlyHunks', () => {
  const patch = [
    'diff --git a/foo.ts b/foo.ts',
    'index 111..222 100644',
    '--- a/foo.ts',
    '+++ b/foo.ts',
    '@@ -1,1 +1,1 @@',
    '-  const a = 1;',
    '+const a = 1;',
    '@@ -10,1 +10,1 @@',
    '-const b = 1;',
    '+const b = 2;',
    '',
  ].join('\n');

  it('drops whitespace-only hunks and keeps the header plus real hunks', () => {
    const result = hideWhitespaceOnlyHunks(patch);
    expect(result.hiddenLineCount).toBe(2);
    expect(result.patch).toContain('diff --git a/foo.ts b/foo.ts');
    expect(result.patch).toContain('-const b = 1;');
    expect(result.patch).not.toContain('-  const a = 1;');
  });

  it('reports nothing hidden when every hunk has real changes', () => {
    const realOnly = [
      'diff --git a/foo.ts b/foo.ts',
      '--- a/foo.ts',
      '+++ b/foo.ts',
      '@@ -1,1 +1,1 @@',
      '-const a = 1;',
      '+const a = 2;',
      '',
    ].join('\n');
    const result = hideWhitespaceOnlyHunks(realOnly);
    expect(result.hiddenLineCount).toBe(0);
    expect(result.patch).toBe(realOnly);
  });

  it('drops every hunk and keeps the header when all hunks are whitespace-only', () => {
    const allWhitespace = [
      'diff --git a/foo.ts b/foo.ts',
      '--- a/foo.ts',
      '+++ b/foo.ts',
      '@@ -1,1 +1,1 @@',
      '-  const a = 1;',
      '+const a = 1;',
      '',
    ].join('\n');
    const result = hideWhitespaceOnlyHunks(allWhitespace);
    // The header-only patch still parses; the hidden count is the only signal
    // that the file changed at all.
    expect(result.hiddenLineCount).toBe(2);
    expect(result.patch).toContain('diff --git a/foo.ts b/foo.ts');
    expect(result.patch).not.toContain('@@');
  });

  it('leaves a patch with no hunks untouched', () => {
    const headerOnly = 'diff --git a/foo.bin b/foo.bin\nBinary files differ\n';
    expect(hideWhitespaceOnlyHunks(headerOnly)).toEqual({
      patch: headerOnly,
      hiddenLineCount: 0,
    });
  });
});
