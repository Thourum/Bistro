import { describe, expect, it } from 'vitest';
import {
  type CursorHunk,
  cursorRange,
  extendCursor,
  initialCursor,
  type LineCursor,
  listCommentableLines,
  moveCursor,
  switchSide,
} from './lineCommentCursor';

// @@ -10,4 +10,3 @@ : context 10, -11, -12, +11, context 13→12
const HUNK: CursorHunk = {
  deletionStart: 10,
  additionStart: 10,
  hunkContent: [
    { type: 'context', lines: 1 },
    { type: 'change', deletions: 2, additions: 1 },
    { type: 'context', lines: 1 },
  ],
};
const SECOND_HUNK: CursorHunk = {
  deletionStart: 40,
  additionStart: 39,
  hunkContent: [{ type: 'change', deletions: 0, additions: 2 }],
};

const at = (side: 'LEFT' | 'RIGHT', line: number): LineCursor => ({
  side,
  anchor: line,
  head: line,
});
const spots = (lines: ReturnType<typeof listCommentableLines>) =>
  lines.map(({ side, line, row }) => `${side}${line}@${row}`);

describe('listCommentableLines', () => {
  it('lists unified rows in display order with context on the right side', () => {
    expect(spots(listCommentableLines([HUNK], 'unified'))).toEqual([
      'RIGHT10@0',
      'LEFT11@1',
      'LEFT12@2',
      'RIGHT11@3',
      'RIGHT12@4',
    ]);
  });

  it('pairs both sides of a split row', () => {
    expect(spots(listCommentableLines([HUNK], 'split'))).toEqual([
      'LEFT10@0',
      'RIGHT10@0',
      'LEFT11@1',
      'RIGHT11@1',
      'LEFT12@2',
      'LEFT13@3',
      'RIGHT12@3',
    ]);
  });
});

describe('cursor movement', () => {
  const unified = listCommentableLines([HUNK, SECOND_HUNK], 'unified');
  const split = listCommentableLines([HUNK], 'split');

  it('starts on the first changed line', () => {
    expect(initialCursor(unified)).toEqual(at('LEFT', 11));
    expect(initialCursor([])).toBeNull();
  });

  it('walks every unified row across sides and clamps at the file end', () => {
    let cursor = at('LEFT', 12);
    cursor = moveCursor(unified, cursor, 'unified', 1);
    expect(cursor).toEqual(at('RIGHT', 11));
    cursor = moveCursor(unified, at('RIGHT', 40), 'unified', 1);
    expect(cursor).toEqual(at('RIGHT', 40));
    expect(moveCursor(unified, at('RIGHT', 10), 'unified', -1)).toEqual(
      at('RIGHT', 10),
    );
  });

  it('stays on its side in split view', () => {
    expect(moveCursor(split, at('LEFT', 12), 'split', 1)).toEqual(
      at('LEFT', 13),
    );
  });

  it('collapses an extended range before moving', () => {
    const ranged: LineCursor = { side: 'RIGHT', anchor: 10, head: 11 };
    expect(moveCursor(unified, ranged, 'unified', 1)).toEqual(at('RIGHT', 12));
  });
});

describe('range extension', () => {
  const unified = listCommentableLines([HUNK, SECOND_HUNK], 'unified');

  it('extends on the anchor side in either direction', () => {
    const down = extendCursor(unified, at('RIGHT', 11), 1);
    expect(cursorRange(down)).toEqual({
      side: 'RIGHT',
      startLine: 11,
      line: 12,
    });
    const up = extendCursor(
      unified,
      extendCursor(unified, at('RIGHT', 11), -1),
      -1,
    );
    expect(cursorRange(up)).toEqual({ side: 'RIGHT', startLine: 10, line: 11 });
  });

  it('never crosses into the next hunk', () => {
    expect(extendCursor(unified, at('RIGHT', 12), 1)).toEqual(at('RIGHT', 12));
  });

  it('reports a single line without a start line', () => {
    expect(cursorRange(at('LEFT', 11))).toEqual({
      side: 'LEFT',
      startLine: null,
      line: 11,
    });
  });
});

describe('switchSide', () => {
  const split = listCommentableLines([HUNK], 'split');

  it('jumps to the nearest row on the other side, preferring the earlier row', () => {
    expect(switchSide(split, at('LEFT', 12), 'RIGHT')).toEqual(at('RIGHT', 11));
  });

  it('is a no-op when already on the target side', () => {
    expect(switchSide(split, at('RIGHT', 11), 'RIGHT')).toEqual(
      at('RIGHT', 11),
    );
  });
});
