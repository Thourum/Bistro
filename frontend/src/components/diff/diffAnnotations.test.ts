import { type FileDiffMetadata, type Hunk, processFile } from '@pierre/diffs';
import { assert, describe, expect, it } from 'vitest';
import type { ReviewDraft, ReviewThread } from '../../types';
import {
  buildOrderedAnnotations,
  draftAnnotationKey,
  draftIdFromKey,
  isDraftAnchored,
  lineExistsOnSide,
} from './diffAnnotations';
import type { ParsedDiffFile } from './diffPathUtils';

function hunk(
  additionStart: number,
  additionCount: number,
  deletionStart = 0,
  deletionCount = 0,
): Hunk {
  return { additionStart, additionCount, deletionStart, deletionCount } as Hunk;
}

function fileDiff(hunks: Hunk[]): FileDiffMetadata {
  return { hunks } as FileDiffMetadata;
}

const file: ParsedDiffFile = {
  id: 'file-a',
  path: 'a.ts',
  oldFile: 'a/a.ts',
  newFile: 'b/a.ts',
  canonicalKey: 'a.ts',
  lang: 'TS',
  additions: 1,
  deletions: 2,
  patch: '',
  lineCount: 0,
};

const thread: ReviewThread = {
  id: 'thread-1',
  pr_id: 'pr-1',
  file_path: 'a.ts',
  line: 20,
  resolved: false,
  outdated: false,
  state_known: true,
};

const draft: ReviewDraft = {
  id: 'draft-1',
  pr_id: 'pr-1',
  path: 'a.ts',
  side: 'LEFT',
  start_line: 0,
  line: 11,
  body: 'why?',
  commit_sha: 'sha',
  created_at: '2026-10-23T00:00:00Z',
};

describe('lineExistsOnSide', () => {
  it('returns true for a line inside a single hunk', () => {
    expect(lineExistsOnSide(fileDiff([hunk(10, 5)]), 12, 'additions')).toBe(
      true,
    );
  });

  it('returns true for the first and last line of a hunk (inclusive range)', () => {
    const diff = fileDiff([hunk(10, 5)]);
    expect(lineExistsOnSide(diff, 10, 'additions')).toBe(true);
    expect(lineExistsOnSide(diff, 14, 'additions')).toBe(true);
  });

  it('returns false for a line just past the end of a hunk', () => {
    expect(lineExistsOnSide(fileDiff([hunk(10, 5)]), 15, 'additions')).toBe(
      false,
    );
  });

  it('returns false for a line before any hunk', () => {
    expect(lineExistsOnSide(fileDiff([hunk(10, 5)]), 9, 'additions')).toBe(
      false,
    );
  });

  it('checks across multiple hunks', () => {
    const diff = fileDiff([hunk(1, 3), hunk(50, 2)]);
    expect(lineExistsOnSide(diff, 2, 'additions')).toBe(true);
    expect(lineExistsOnSide(diff, 51, 'additions')).toBe(true);
    expect(lineExistsOnSide(diff, 10, 'additions')).toBe(false);
  });

  it('returns false when the file has no hunks', () => {
    expect(lineExistsOnSide(fileDiff([]), 1, 'additions')).toBe(false);
  });

  it('checks deletion-side lines against the old file range', () => {
    const parsed = processFile(
      'diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n@@ -10,2 +10,1 @@\n-old\n-gone\n+new\n',
    );
    if (parsed === undefined) throw new Error('expected a parsed diff');
    expect(lineExistsOnSide(parsed, 11, 'deletions')).toBe(true);
    expect(lineExistsOnSide(parsed, 12, 'deletions')).toBe(false);
  });
});

// Old lines 10-11 and new lines 18-22 are in the diff.
const parsed = processFile(
  'diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n@@ -10,2 +18,5 @@\n-old\n-gone\n+one\n+two\n+three\n+four\n+five\n',
);
assert(parsed !== undefined, 'expected a parsed diff');
const anchorable = { file, fileDiff: parsed };

describe('isDraftAnchored', () => {
  it('anchors only drafts whose path and side line are in the diff', () => {
    expect(isDraftAnchored(draft, [anchorable])).toBe(true);
    expect(isDraftAnchored({ ...draft, line: 12 }, [anchorable])).toBe(false);
    expect(isDraftAnchored({ ...draft, side: 'RIGHT' }, [anchorable])).toBe(
      false,
    );
    expect(isDraftAnchored({ ...draft, path: 'b.ts' }, [anchorable])).toBe(
      false,
    );
  });
});

describe('buildOrderedAnnotations', () => {
  it('orders offset left/right anchors by unified row across hunks and files', () => {
    const offsetDiff = processFile(
      'diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n@@ -10,1 +1,1 @@\n-old\n+new\n@@ -30,1 +15,1 @@\n-later old\n+later new\n',
    );
    assert(offsetDiff !== undefined, 'expected a parsed diff');
    const secondFile = { ...file, id: 'file-b', path: 'b.ts' };
    const entries = buildOrderedAnnotations(
      [
        { file, fileDiff: offsetDiff },
        { file: secondFile, fileDiff: offsetDiff },
      ],
      [
        { ...thread, id: 'later-right', line: 15 },
        { ...thread, id: 'second-file', file_path: 'b.ts', line: 1 },
        { ...thread, id: 'first-right', line: 1 },
      ],
      [
        { ...draft, id: 'later-left', line: 30 },
        { ...draft, id: 'first-left', line: 10 },
      ],
    );
    expect(entries.map(({ key }) => key)).toEqual([
      draftAnnotationKey('first-left'),
      'first-right',
      draftAnnotationKey('later-left'),
      'later-right',
      'second-file',
    ]);
    expect(entries.map(({ fileIndex }) => fileIndex)).toEqual([0, 0, 0, 0, 1]);
  });

  it('orders left context anchors at their unified rows with changing offsets', () => {
    const contextDiff = processFile(
      'diff --git a/a.ts b/a.ts\n--- a/a.ts\n+++ b/a.ts\n@@ -10,5 +1,4 @@\n before\n-old\n-gone\n+new\n after\n last\n@@ -30,3 +20,4 @@\n later before\n-later old\n+later new\n+extra\n later after\n',
    );
    assert(contextDiff !== undefined, 'expected a parsed diff');
    const entries = buildOrderedAnnotations(
      [{ file, fileDiff: contextDiff }],
      [
        { ...thread, id: 'missing', line: 5 },
        ...[1, 2, 3, 4, 20, 21, 22, 23].map((line) => ({
          ...thread,
          id: `right-${line}`,
          line,
        })),
      ],
      [
        { ...draft, id: 'after-z', line: 13 },
        { ...draft, id: 'after-a', line: 13 },
        ...[10, 14, 30, 32].map((line) => ({
          ...draft,
          id: `left-${line}`,
          line,
        })),
      ],
    );
    expect(entries.map(({ key }) => key)).toEqual([
      'right-1',
      draftAnnotationKey('left-10'),
      'right-2',
      'right-3',
      draftAnnotationKey('after-a'),
      draftAnnotationKey('after-z'),
      'right-4',
      draftAnnotationKey('left-14'),
      'right-20',
      draftAnnotationKey('left-30'),
      'right-21',
      'right-22',
      'right-23',
      draftAnnotationKey('left-32'),
      'missing',
    ]);
  });

  it('puts unmatched rows last with line/key ties and preserves filtering', () => {
    const entries = buildOrderedAnnotations(
      [anchorable],
      [
        { ...thread, id: 'row-z', line: 18 },
        { ...thread, id: 'missing-z', line: 1 },
        { ...thread, id: 'missing-b', line: 2 },
        { ...thread, id: 'row-a', line: 18 },
        { ...thread, id: 'missing-a', line: 1 },
        { ...thread, id: 'outdated', outdated: true },
        { ...thread, id: 'unknown', state_known: false },
        { ...thread, id: 'no-line', line: 0 },
        { ...thread, id: 'other-path', file_path: 'b.ts' },
      ],
      [
        { ...draft, id: 'same-row', side: 'RIGHT', line: 18 },
        { ...draft, id: 'outside-hunks', line: 1 },
        { ...draft, id: 'other-path', path: 'b.ts' },
      ],
    );
    expect(entries.map(({ key }) => key)).toEqual([
      draftAnnotationKey('same-row'),
      'row-a',
      'row-z',
      'missing-a',
      'missing-z',
      'missing-b',
    ]);
  });

  it('orders anchored drafts with threads and keys them apart', () => {
    const unanchored = { ...draft, id: 'draft-2', line: 40 };
    const entries = buildOrderedAnnotations(
      [anchorable],
      [thread],
      [draft, unanchored],
    );
    expect(entries.map((entry) => entry.key)).toEqual([
      draftAnnotationKey(draft.id),
      thread.id,
    ]);
    expect(draftIdFromKey(entries[0]?.key ?? null)).toBe(draft.id);
    expect(draftIdFromKey(thread.id)).toBeNull();
  });
});
