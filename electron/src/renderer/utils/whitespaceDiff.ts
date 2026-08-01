/**
 * Whitespace-only hunk detection over raw unified-diff text.
 *
 * `@pierre/diffs` has no ignore-whitespace option (verified against 1.2.11
 * typings), so the reviewer-facing `w` toggle filters the patch text before it
 * reaches the library's parser.
 */

export interface HiddenWhitespaceResult {
  /** The patch with whitespace-only hunks removed. */
  patch: string;
  /** How many `+`/`-` lines were dropped (0 when nothing was hidden). */
  hiddenLineCount: number;
}

const HUNK_HEADER = /^@@ /;

function isChangeLine(line: string): boolean {
  return (
    (line.startsWith('+') || line.startsWith('-')) &&
    !line.startsWith('+++') &&
    !line.startsWith('---')
  );
}

function withoutWhitespace(lines: readonly string[]): string {
  return lines.map((line) => line.slice(1).replace(/\s+/g, '')).join('\n');
}

/**
 * True when every changed line in the hunk is a whitespace-only rewrite of its
 * counterpart: the removed and added sides are identical once all whitespace is
 * stripped, and both sides are non-empty (a pure addition or deletion is a real
 * change, not reformatting).
 */
export function isWhitespaceOnlyHunk(hunkLines: readonly string[]): boolean {
  const removed = hunkLines.filter(
    (line) => line.startsWith('-') && isChangeLine(line),
  );
  const added = hunkLines.filter(
    (line) => line.startsWith('+') && isChangeLine(line),
  );
  if (removed.length === 0 || added.length === 0) return false;
  return withoutWhitespace(removed) === withoutWhitespace(added);
}

function countChangeLines(lines: readonly string[]): number {
  return lines.filter(isChangeLine).length;
}

/**
 * Remove whitespace-only hunks from a single-file patch.
 *
 * The file preamble (`diff --git`, `index`, `---`/`+++`) is always kept so the
 * file still renders with its header. When *every* hunk is whitespace-only the
 * patch is returned unchanged: an empty diff body would render as a blank file
 * with no way to tell reformatting from "no changes".
 */
export function hideWhitespaceOnlyHunks(patch: string): HiddenWhitespaceResult {
  const lines = patch.split('\n');
  const firstHunk = lines.findIndex((line) => HUNK_HEADER.test(line));
  if (firstHunk === -1) return { patch, hiddenLineCount: 0 };

  const preamble = lines.slice(0, firstHunk);
  const hunks: string[][] = [];
  for (const line of lines.slice(firstHunk)) {
    if (HUNK_HEADER.test(line)) {
      hunks.push([line]);
      continue;
    }
    hunks[hunks.length - 1]?.push(line);
  }

  const kept = hunks.filter((hunk) => !isWhitespaceOnlyHunk(hunk));
  if (kept.length === hunks.length || kept.length === 0) {
    return { patch, hiddenLineCount: 0 };
  }

  const hiddenLineCount = hunks
    .filter((hunk) => isWhitespaceOnlyHunk(hunk))
    .reduce((total, hunk) => total + countChangeLines(hunk), 0);

  return {
    patch: [...preamble, ...kept.flat()].join('\n'),
    hiddenLineCount,
  };
}
