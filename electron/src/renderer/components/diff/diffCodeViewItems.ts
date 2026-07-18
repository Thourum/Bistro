import {
  type CodeViewItem,
  type DiffLineAnnotation,
  processFile,
} from '@pierre/diffs';
import type { Comment, ReviewThread } from '../../types';
import { buildFileAnnotations } from './diffAnnotations';
import type { ParsedDiffFile } from './diffPathUtils';

export interface ParsedCodeViewFile {
  file: ParsedDiffFile;
  fileDiff: NonNullable<ReturnType<typeof processFile>>;
}

export interface UnrenderableDiffFile {
  file: ParsedDiffFile;
  reason: 'parse-failed';
}

export interface CodeViewFileParseResult {
  parsedFiles: ParsedCodeViewFile[];
  unrenderableFiles: UnrenderableDiffFile[];
}

export function parseCodeViewFiles(
  files: readonly ParsedDiffFile[],
): CodeViewFileParseResult {
  const parsed = files.map((file) => ({
    file,
    fileDiff: processFile(file.patch),
  }));
  return {
    parsedFiles: parsed.flatMap(({ file, fileDiff }) =>
      fileDiff === undefined ? [] : [{ file, fileDiff }],
    ),
    unrenderableFiles: parsed.flatMap(({ file, fileDiff }) =>
      fileDiff === undefined ? [{ file, reason: 'parse-failed' as const }] : [],
    ),
  };
}

export function buildCodeViewItems(
  parsedCodeViewFiles: readonly ParsedCodeViewFile[],
  threads: readonly ReviewThread[],
  commentsByThread: ReadonlyMap<string, readonly Comment[]>,
  activeAnnotationKey: string | null,
  replyingToThreadId: string | null,
): CodeViewItem<ReviewThread>[] {
  return parsedCodeViewFiles.map(({ file, fileDiff }) => {
    const annotations = buildFileAnnotations(file, threads);
    return {
      id: file.id,
      type: 'diff',
      fileDiff,
      annotations,
      version: buildItemVersion(
        file,
        annotations,
        commentsByThread,
        activeAnnotationKey,
        replyingToThreadId,
      ),
    };
  });
}

export function groupCommentsByThread(
  comments: readonly Comment[],
): Map<string, Comment[]> {
  const grouped = new Map<string, Comment[]>();
  for (const comment of comments) {
    if (comment.thread_id === undefined) continue;
    const existing = grouped.get(comment.thread_id);
    if (existing === undefined) {
      grouped.set(comment.thread_id, [comment]);
    } else {
      existing.push(comment);
    }
  }
  return grouped;
}

function buildItemVersion(
  file: ParsedDiffFile,
  annotations: readonly DiffLineAnnotation<ReviewThread>[],
  commentsByThread: ReadonlyMap<string, readonly Comment[]>,
  activeAnnotationKey: string | null,
  replyingToThreadId: string | null,
): number {
  const content = [
    file.patch,
    ...annotations.map((annotation) => {
      const thread = annotation.metadata;
      if (thread === undefined) {
        return `${annotation.side}:${annotation.lineNumber}`;
      }
      const comments = commentsByThread.get(thread.id) ?? [];
      return [
        annotation.side,
        annotation.lineNumber,
        thread.id,
        thread.state_known,
        thread.resolved,
        thread.outdated,
        activeAnnotationKey === thread.id,
        replyingToThreadId === thread.id,
        ...comments.map((comment) =>
          [
            comment.id,
            comment.body,
            comment.severity,
            comment.bot_vendor,
            comment.classification_source,
          ].join(':'),
        ),
      ].join(':');
    }),
  ].join('|');
  return hashString(content);
}

function hashString(value: string): number {
  let hash = 0;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) | 0;
  }
  return hash;
}
