import type { DiffLineAnnotation, FileDiffMetadata } from '@pierre/diffs';
import type { ReviewThread } from '../../types';
import type { ParsedDiffFile } from './diffPathUtils';

/** A flattened, ordered annotation entry used for j/k keyboard navigation. */
export interface AnnotationEntry {
  /** Unique key = thread id. */
  key: string;
  threadId: string;
  /** Index of the owning file in the parsed file list. */
  fileIndex: number;
  fileId: string;
  line: number;
}

export function hasCurrentInlineAnchor(thread: ReviewThread): boolean {
  return thread.line > 0 && thread.state_known && !thread.outdated;
}

export function buildOrderedAnnotations(
  files: readonly ParsedDiffFile[],
  threads: readonly ReviewThread[],
): AnnotationEntry[] {
  return files.flatMap((file, fileIndex) =>
    threads
      .filter(
        (thread) =>
          thread.file_path === file.path && hasCurrentInlineAnchor(thread),
      )
      .sort(
        (left, right) =>
          left.line - right.line || left.id.localeCompare(right.id),
      )
      .map((thread) => ({
        key: thread.id,
        threadId: thread.id,
        fileIndex,
        fileId: file.id,
        line: thread.line,
      })),
  );
}

export function buildFileAnnotations(
  file: ParsedDiffFile,
  threads: readonly ReviewThread[],
): DiffLineAnnotation<ReviewThread>[] {
  return threads
    .filter(
      (thread) =>
        thread.file_path === file.path && hasCurrentInlineAnchor(thread),
    )
    .map((thread) => ({
      side: 'additions',
      lineNumber: thread.line,
      metadata: thread,
    }));
}

/**
 * True when `lineNumber` falls inside the additions side of one of the
 * file's parsed hunks. `ReviewThread.line` doesn't record which local hunk it
 * anchors to (nor which side), so a thread can legitimately point at a line
 * our own re-parsed patch doesn't cover — e.g. after a force-push, or when
 * anchoring landed on the deletions side. CodeView.scrollTo has no
 * "best effort" mode for that case: it warns and no-ops. Callers should skip
 * the CodeView-native scroll (and fall back to the annotation card's DOM
 * scrollIntoView) when this returns false.
 */
export function lineExistsInAdditions(
  fileDiff: FileDiffMetadata,
  lineNumber: number,
): boolean {
  return fileDiff.hunks.some(
    (hunk) =>
      lineNumber >= hunk.additionStart &&
      lineNumber < hunk.additionStart + hunk.additionCount,
  );
}
