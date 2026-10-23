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

function fileEntries(
  file: ParsedDiffFile,
  fileIndex: number,
  threads: readonly ReviewThread[],
  drafts: readonly ReviewDraft[],
): AnnotationEntry[] {
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
        side: toAnnotationSide(draft.side),
      }),
    );
  return [...threadEntries, ...draftEntries].sort(
    (left, right) =>
      left.line - right.line || left.key.localeCompare(right.key),
  );
}

export function buildOrderedAnnotations(
  files: readonly ParsedDiffFile[],
  threads: readonly ReviewThread[],
  drafts: readonly ReviewDraft[] = [],
): AnnotationEntry[] {
  return files.flatMap((file, fileIndex) =>
    fileEntries(file, fileIndex, threads, drafts),
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
        side: toAnnotationSide(draft.side),
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
