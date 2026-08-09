import type { FileDiffMetadata, Hunk } from '@pierre/diffs';
import { describe, expect, it } from 'vitest';
import { lineExistsInAdditions } from './diffAnnotations';

function hunk(additionStart: number, additionCount: number): Hunk {
  return { additionStart, additionCount } as Hunk;
}

function fileDiff(hunks: Hunk[]): FileDiffMetadata {
  return { hunks } as FileDiffMetadata;
}

describe('lineExistsInAdditions', () => {
  it('returns true for a line inside a single hunk', () => {
    expect(lineExistsInAdditions(fileDiff([hunk(10, 5)]), 12)).toBe(true);
  });

  it('returns true for the first and last line of a hunk (inclusive range)', () => {
    const diff = fileDiff([hunk(10, 5)]);
    expect(lineExistsInAdditions(diff, 10)).toBe(true);
    expect(lineExistsInAdditions(diff, 14)).toBe(true);
  });

  it('returns false for a line just past the end of a hunk', () => {
    expect(lineExistsInAdditions(fileDiff([hunk(10, 5)]), 15)).toBe(false);
  });

  it('returns false for a line before any hunk', () => {
    expect(lineExistsInAdditions(fileDiff([hunk(10, 5)]), 9)).toBe(false);
  });

  it('checks across multiple hunks', () => {
    const diff = fileDiff([hunk(1, 3), hunk(50, 2)]);
    expect(lineExistsInAdditions(diff, 2)).toBe(true);
    expect(lineExistsInAdditions(diff, 51)).toBe(true);
    expect(lineExistsInAdditions(diff, 10)).toBe(false);
  });

  it('returns false when the file has no hunks', () => {
    expect(lineExistsInAdditions(fileDiff([]), 1)).toBe(false);
  });
});
