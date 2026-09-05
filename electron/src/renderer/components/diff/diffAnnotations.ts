import type { DiffLineAnnotation } from '@pierre/diffs';
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
