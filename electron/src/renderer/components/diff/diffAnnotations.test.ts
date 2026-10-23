import { type FileDiffMetadata, type Hunk, processFile } from '@pierre/diffs';
import { describe, expect, it } from 'vitest';
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
const anchorable = { file, fileDiff: fileDiff([hunk(18, 5, 10, 2)]) };

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
