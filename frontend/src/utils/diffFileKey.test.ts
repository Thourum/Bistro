import { expect, it } from 'vitest';
import { buildFileKeys, canonicalFileKey, findFileByKey } from './diffFileKey';

it('round-trips duplicate paths without colliding with a literal ordinal-looking path', () => {
  const first = { oldFile: 'a/dup.ts', newFile: 'b/dup.ts' };
  const second = { oldFile: 'a/dup.ts', newFile: 'b/dup.ts' };
  const literal = { oldFile: 'a/dup.ts::1', newFile: 'b/dup.ts::1' };
  const files = [first, second, literal];

  const keys = buildFileKeys(files);
  expect(new Set(keys).size).toBe(files.length);
  for (const [index, file] of files.entries()) {
    expect(findFileByKey(files, keys[index] ?? '')).toBe(file);
  }
});

it('does not resolve an ambiguous unsuffixed path or a key outside the list', () => {
  const files = [
    { oldFile: 'a/dup.ts', newFile: 'b/dup.ts' },
    { oldFile: 'a/dup.ts', newFile: 'b/dup.ts' },
  ];

  expect(findFileByKey(files, 'dup.ts')).toBeUndefined();
  expect(findFileByKey(files, 'dup.ts::2')).toBeUndefined();
  expect(
    canonicalFileKey({ oldFile: 'a/dup.ts', newFile: 'b/dup.ts' }, files),
  ).toBe('');
});
