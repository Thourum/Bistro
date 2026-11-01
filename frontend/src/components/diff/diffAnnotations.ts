import type {
  AnnotationSide,
  DiffLineAnnotation,
  FileDiffMetadata,
} from '@pierre/diffs';
import { isDraftOutdated } from '../../services/reviewDrafts';
import type { ReviewDraft, ReviewThread } from '../../types';
import {
  cursorRange,
  type LineCommentState,
  listCommentableLines,
  toAnnotationSide,
} from '../../utils/lineCommentCursor';
import type { ParsedDiffFile } from './diffPathUtils';

const DRAFT_KEY_PREFIX = 'draft:';
export const COMPOSER_ANNOTATION_KEY = 'line-comment-composer';

export type DiffAnnotationMeta =
  | { kind: 'thread'; key: string; thread: ReviewThread }
  | { kind: 'draft'; key: string; draft: ReviewDraft; isOutdated: boolean }
  | { kind: 'composer'; key: string; lineComment: LineCommentState };

export function draftAnnotationKey(draftId: string): string {
  return `${DRAFT_KEY_PREFIX}${draftId}`;
}

export function draftIdFromKey(key: string | null): string | null {
  return key?.startsWith(DRAFT_KEY_PREFIX)
    ? key.slice(DRAFT_KEY_PREFIX.length)
    : null;
}

/** A flattened, ordered annotation entry used for n/p keyboard navigation. */
export interface AnnotationEntry {
  /** Thread id, or draftAnnotationKey(draft.id). */
  key: string;
  /** Index of the owning file in the parsed file list. */
  fileIndex: number;
  fileId: string;
  line: number;
  side: AnnotationSide;
}

export function hasCurrentInlineAnchor(thread: ReviewThread): boolean {
  return thread.line > 0 && thread.state_known && !thread.outdated;
}

function buildAnnotationRows(
  fileDiff: FileDiffMetadata,
): ReadonlyMap<string, number> {
  const rows = new Map(
    listCommentableLines(fileDiff.hunks, 'unified').map(
      ({ side, line, row }) => [`${toAnnotationSide(side)}:${line}`, row],
    ),
  );
  // Unified cursors use RIGHT for context, but cards can anchor on either side.
  for (const hunk of fileDiff.hunks) {
    for (const block of hunk.hunkContent) {
      if (block.type !== 'context') continue;
      const oldStart =
        hunk.deletionStart + block.deletionLineIndex - hunk.deletionLineIndex;
      const newStart =
        hunk.additionStart + block.additionLineIndex - hunk.additionLineIndex;
      for (let offset = 0; offset < block.lines; offset += 1) {
        const row = rows.get(`additions:${newStart + offset}`);
        if (row !== undefined) rows.set(`deletions:${oldStart + offset}`, row);
      }
    }
  }
  return rows;
}

function fileEntries(
  { file, fileDiff }: AnchorableDiffFile,
  fileIndex: number,
  threads: readonly ReviewThread[],
  drafts: readonly ReviewDraft[],
): AnnotationEntry[] {
  const rows = buildAnnotationRows(fileDiff);
  const threadEntries = threads
    .filter(
      (thread) =>
        thread.file_path === file.path && hasCurrentInlineAnchor(thread),
    )
    .map(
      (thread): AnnotationEntry => ({
        key: thread.id,
        fileIndex,
        fileId: file.id,
        line: thread.line,
        side: 'additions',
      }),
    );
  const draftEntries = drafts
    .filter((draft) => draft.path === file.path)
    .map(
      (draft): AnnotationEntry => ({
        key: draftAnnotationKey(draft.id),
        fileIndex,
        fileId: file.id,
        line: draft.line,
        side: toAnnotationSide(draft.side === 'LEFT' ? 'LEFT' : 'RIGHT'),
      }),
    );
  return [...threadEntries, ...draftEntries].sort(
    (left, right) =>
      (rows.get(`${left.side}:${left.line}`) ?? Number.POSITIVE_INFINITY) -
        (rows.get(`${right.side}:${right.line}`) ?? Number.POSITIVE_INFINITY) ||
      left.line - right.line ||
      left.key.localeCompare(right.key),
  );
}

/** A parsed file plus the hunks its annotations anchor to. */
export interface AnchorableDiffFile {
  file: ParsedDiffFile;
  fileDiff: FileDiffMetadata;
}

/**
 * True when the draft renders a card: its path is in the diff and its line is
 * inside a hunk on its side. Other drafts are kept but can't be reached here.
 */
export function isDraftAnchored(
  draft: ReviewDraft,
  files: readonly AnchorableDiffFile[],
): boolean {
  const parsed = files.find(({ file }) => file.path === draft.path);
  return (
    parsed !== undefined &&
    lineExistsOnSide(
      parsed.fileDiff,
      draft.line,
      toAnnotationSide(draft.side === 'LEFT' ? 'LEFT' : 'RIGHT'),
    )
  );
}

export function buildOrderedAnnotations(
  files: readonly AnchorableDiffFile[],
  threads: readonly ReviewThread[],
  drafts: readonly ReviewDraft[] = [],
): AnnotationEntry[] {
  const anchoredDrafts = drafts.filter((draft) =>
    isDraftAnchored(draft, files),
  );
  return files.flatMap((parsed, fileIndex) =>
    fileEntries(parsed, fileIndex, threads, anchoredDrafts),
  );
}

export interface FileAnnotationSources {
  threads: readonly ReviewThread[];
  drafts: readonly ReviewDraft[];
  headSha: string;
  lineComment: LineCommentState | null;
}

export function buildFileAnnotations(
  file: ParsedDiffFile,
  { threads, drafts, headSha, lineComment }: FileAnnotationSources,
): DiffLineAnnotation<DiffAnnotationMeta>[] {
  const threadAnnotations = threads
    .filter(
      (thread) =>
        thread.file_path === file.path && hasCurrentInlineAnchor(thread),
    )
    .map(
      (thread): DiffLineAnnotation<DiffAnnotationMeta> => ({
        side: 'additions',
        lineNumber: thread.line,
        metadata: { kind: 'thread', key: thread.id, thread },
      }),
    );
  // The draft being edited is replaced by the composer at its position.
  const editingDraftId = lineComment?.composer?.draftId ?? null;
  const draftAnnotations = drafts
    .filter((draft) => draft.path === file.path && draft.id !== editingDraftId)
    .map(
      (draft): DiffLineAnnotation<DiffAnnotationMeta> => ({
        side: toAnnotationSide(draft.side === 'LEFT' ? 'LEFT' : 'RIGHT'),
        lineNumber: draft.line,
        metadata: {
          kind: 'draft',
          key: draftAnnotationKey(draft.id),
          draft,
          isOutdated: isDraftOutdated(draft, headSha),
        },
      }),
    );
  if (lineComment?.composer == null || lineComment.fileId !== file.id)
    return [...threadAnnotations, ...draftAnnotations];
  const range = cursorRange(lineComment.cursor);
  return [
    ...threadAnnotations,
    ...draftAnnotations,
    {
      side: toAnnotationSide(range.side),
      lineNumber: range.line,
      metadata: { kind: 'composer', key: COMPOSER_ANNOTATION_KEY, lineComment },
    },
  ];
}

/**
 * True when `lineNumber` falls inside `side` of one of the file's parsed
 * hunks (the new-file range for additions, the old-file range for deletions).
 * `ReviewThread.line` doesn't record which local hunk it anchors to (nor which
 * side), so a thread can legitimately point at a line our own re-parsed patch
 * doesn't cover — e.g. after a force-push, or when anchoring landed on the
 * other side. CodeView.scrollTo has no "best effort" mode for that case: it
 * warns and no-ops. Callers should skip the CodeView-native scroll (and fall
 * back to the annotation card's DOM scrollIntoView) when this returns false.
 */
export function lineExistsOnSide(
  fileDiff: FileDiffMetadata,
  lineNumber: number,
  side: AnnotationSide,
): boolean {
  return fileDiff.hunks.some((hunk) => {
    const start =
      side === 'additions' ? hunk.additionStart : hunk.deletionStart;
    const count =
      side === 'additions' ? hunk.additionCount : hunk.deletionCount;
    return lineNumber >= start && lineNumber < start + count;
  });
}
