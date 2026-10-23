/**
 * Keyboard cursor for picking line-comment targets in a diff. Pure: the
 * comment overlay stores a LineCommentState and keybinding actions replace it.
 * Coordinates are GitHub's: LEFT = base file line, RIGHT = head file line.
 */

export type ReviewSide = 'LEFT' | 'RIGHT';
export type DiffLayout = 'unified' | 'split';

/** The parts of a parsed @pierre/diffs hunk the cursor needs. */
export interface CursorHunk {
  deletionStart: number;
  additionStart: number;
  hunkContent: readonly (
    | { type: 'context'; lines: number }
    | { type: 'change'; deletions: number; additions: number }
  )[];
}

export interface CommentableLine {
  side: ReviewSide;
  line: number;
  /**
   * Ordinal for ordering and side-switch distance within one file, not
   * @pierre/diffs' rendered row index; split view gives both sides of a row
   * the same number.
   */
  row: number;
  hunkIndex: number;
  isChange: boolean;
}

/** anchor/head are line numbers on `side`; anchor stays put while extending. */
export interface LineCursor {
  side: ReviewSide;
  anchor: number;
  head: number;
}

export interface CommentRange {
  side: ReviewSide;
  /** Null for a single-line comment. */
  startLine: number | null;
  line: number;
}

export interface LineCommentState {
  prId: string;
  fileId: string;
  path: string;
  /** PR head when the overlay opened; becomes the draft's commit_sha. */
  headSha: string;
  layout: DiffLayout;
  lines: readonly CommentableLine[];
  cursor: LineCursor;
  /** Set once Enter opens the composer; draftId is non-null when editing. */
  composer: { draftId: string | null; initialBody: string } | null;
}

export function listCommentableLines(
  hunks: readonly CursorHunk[],
  layout: DiffLayout,
): CommentableLine[] {
  const lines: CommentableLine[] = [];
  let row = 0;
  hunks.forEach((hunk, hunkIndex) => {
    let oldLine = hunk.deletionStart;
    let newLine = hunk.additionStart;
    for (const block of hunk.hunkContent) {
      if (block.type === 'context') {
        for (let offset = 0; offset < block.lines; offset += 1) {
          if (layout === 'split')
            lines.push({
              side: 'LEFT',
              line: oldLine,
              row,
              hunkIndex,
              isChange: false,
            });
          lines.push({
            side: 'RIGHT',
            line: newLine,
            row,
            hunkIndex,
            isChange: false,
          });
          oldLine += 1;
          newLine += 1;
          row += 1;
        }
        continue;
      }
      const blockStart = row;
      // Unified view shows a change block's deletions above its additions.
      const additionsStart =
        layout === 'split' ? blockStart : blockStart + block.deletions;
      for (let offset = 0; offset < block.deletions; offset += 1)
        lines.push({
          side: 'LEFT',
          line: oldLine + offset,
          row: blockStart + offset,
          hunkIndex,
          isChange: true,
        });
      for (let offset = 0; offset < block.additions; offset += 1)
        lines.push({
          side: 'RIGHT',
          line: newLine + offset,
          row: additionsStart + offset,
          hunkIndex,
          isChange: true,
        });
      oldLine += block.deletions;
      newLine += block.additions;
      row =
        layout === 'split'
          ? blockStart + Math.max(block.deletions, block.additions)
          : additionsStart + block.additions;
    }
  });
  return lines.sort(
    (left, right) =>
      left.row - right.row ||
      (left.side === right.side ? 0 : left.side === 'LEFT' ? -1 : 1),
  );
}

function collapsedAt(entry: CommentableLine): LineCursor {
  return { side: entry.side, anchor: entry.line, head: entry.line };
}

function stepWithin(
  candidates: readonly CommentableLine[],
  cursor: { side: ReviewSide; line: number },
  delta: 1 | -1,
): CommentableLine | undefined {
  const index = candidates.findIndex(
    (entry) => entry.side === cursor.side && entry.line === cursor.line,
  );
  if (index < 0) return candidates[0];
  const next = Math.min(Math.max(index + delta, 0), candidates.length - 1);
  return candidates[next];
}

export function initialCursor(
  lines: readonly CommentableLine[],
): LineCursor | null {
  const first = lines.find((entry) => entry.isChange) ?? lines[0];
  return first === undefined ? null : collapsedAt(first);
}

/** Moves the head one row and collapses the range; split view keeps the side. */
export function moveCursor(
  lines: readonly CommentableLine[],
  cursor: LineCursor,
  layout: DiffLayout,
  delta: 1 | -1,
): LineCursor {
  const candidates =
    layout === 'split'
      ? lines.filter((entry) => entry.side === cursor.side)
      : lines;
  const next = stepWithin(
    candidates,
    { side: cursor.side, line: cursor.head },
    delta,
  );
  return next === undefined ? cursor : collapsedAt(next);
}

/** Moves only the head, staying on the anchor's side and hunk (GitHub rule). */
export function extendCursor(
  lines: readonly CommentableLine[],
  cursor: LineCursor,
  delta: 1 | -1,
): LineCursor {
  const anchor = lines.find(
    (entry) => entry.side === cursor.side && entry.line === cursor.anchor,
  );
  if (anchor === undefined) return cursor;
  const candidates = lines.filter(
    (entry) =>
      entry.side === cursor.side && entry.hunkIndex === anchor.hunkIndex,
  );
  const next = stepWithin(
    candidates,
    { side: cursor.side, line: cursor.head },
    delta,
  );
  return next === undefined ? cursor : { ...cursor, head: next.line };
}

/** Split view: jump to the nearest row on `target`; ties go to the earlier row. */
export function switchSide(
  lines: readonly CommentableLine[],
  cursor: LineCursor,
  target: ReviewSide,
): LineCursor {
  if (cursor.side === target) return cursor;
  const current = lines.find(
    (entry) => entry.side === cursor.side && entry.line === cursor.head,
  );
  const others = lines.filter((entry) => entry.side === target);
  const [first] = others;
  if (current === undefined || first === undefined) return cursor;
  const nearest = others.reduce(
    (best, entry) =>
      Math.abs(entry.row - current.row) < Math.abs(best.row - current.row)
        ? entry
        : best,
    first,
  );
  return collapsedAt(nearest);
}

export function cursorRange(cursor: LineCursor): CommentRange {
  const start = Math.min(cursor.anchor, cursor.head);
  const end = Math.max(cursor.anchor, cursor.head);
  return {
    side: cursor.side,
    startLine: start === end ? null : start,
    line: end,
  };
}

export function toAnnotationSide(side: ReviewSide): 'deletions' | 'additions' {
  return side === 'LEFT' ? 'deletions' : 'additions';
}
