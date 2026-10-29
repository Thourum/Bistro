import {
  type CodeViewItem,
  type DiffLineAnnotation,
  processFile,
} from '@pierre/diffs';
import type {
  Comment,
  ExpertiseExpert,
  ExpertiseFileData,
  ReviewDraft,
  ReviewThread,
} from '../../types';
import { isGeneratedFile } from '../../utils/generatedFiles';
import {
  cursorRange,
  type LineCommentState,
} from '../../utils/lineCommentCursor';
import {
  buildFileAnnotations,
  type DiffAnnotationMeta,
} from './diffAnnotations';
import type { ParsedDiffFile } from './diffPathUtils';

export interface ParsedCodeViewFile {
  file: ParsedDiffFile;
  fileDiff: NonNullable<ReturnType<typeof processFile>>;
  patchVersion: number;
}

export type UnrenderableDiffFile =
  | { file: ParsedDiffFile; reason: 'parse-failed' | 'binary' }
  | { file: ParsedDiffFile; reason: 'too-large'; patchBytes: number };

const BINARY_MARKERS = [/^Binary files .* differ$/m, /^GIT binary patch$/m];

/**
 * True for patches git emits without text content. They parse to zero hunks,
 * so CodeView would render them as an empty file.
 */
export function isBinaryPatch(patch: string): boolean {
  return BINARY_MARKERS.some((marker) => marker.test(patch));
}

export interface CodeViewFileParseResult {
  parsedFiles: ParsedCodeViewFile[];
  unrenderableFiles: UnrenderableDiffFile[];
}

const NO_OVERSIZED_FILES: ReadonlyMap<string, number> = new Map();

export function parseCodeViewFiles(
  files: readonly ParsedDiffFile[],
  oversizedPatchBytes: ReadonlyMap<string, number> = NO_OVERSIZED_FILES,
): CodeViewFileParseResult {
  const parsedFiles: ParsedCodeViewFile[] = [];
  const unrenderableFiles: UnrenderableDiffFile[] = [];
  for (const file of files) {
    // The engine stores no patch for files over its per-file cap.
    const patchBytes = oversizedPatchBytes.get(file.path);
    if (patchBytes !== undefined) {
      unrenderableFiles.push({ file, reason: 'too-large', patchBytes });
      continue;
    }
    // Binary patches parse "successfully" into zero hunks, so classify first.
    const isBinary = isBinaryPatch(file.patch);
    const fileDiff = isBinary ? undefined : processFile(file.patch);
    if (fileDiff === undefined) {
      unrenderableFiles.push({
        file,
        reason: isBinary ? 'binary' : 'parse-failed',
      });
      continue;
    }
    parsedFiles.push({ file, fileDiff, patchVersion: hashString(file.patch) });
  }
  return { parsedFiles, unrenderableFiles };
}

export interface CodeViewItemsOptions {
  parsedCodeViewFiles: readonly ParsedCodeViewFile[];
  threads: readonly ReviewThread[];
  drafts: readonly ReviewDraft[];
  /** PR head; drafts anchored to another commit render as outdated. */
  headSha: string;
  lineComment: LineCommentState | null;
  commentsByThread: ReadonlyMap<string, readonly Comment[]>;
  activeAnnotationKey: string | null;
  replyingToThreadId: string | null;
  expertiseByPath: ReadonlyMap<string, ExpertiseFileData>;
  expertiseError: string | null;
  collapseOverrides?: readonly string[];
}

export function buildCodeViewItems({
  parsedCodeViewFiles,
  threads,
  drafts,
  headSha,
  lineComment,
  commentsByThread,
  activeAnnotationKey,
  replyingToThreadId,
  expertiseByPath,
  expertiseError,
  collapseOverrides = [],
}: CodeViewItemsOptions): CodeViewItem<DiffAnnotationMeta>[] {
  return parsedCodeViewFiles.map(({ file, fileDiff, patchVersion }) => {
    const annotations = buildFileAnnotations(file, {
      threads,
      drafts,
      headSha,
      lineComment,
    });
    const collapsed = isFileCollapsed(file, collapseOverrides);
    return {
      id: file.id,
      type: 'diff',
      fileDiff,
      annotations,
      collapsed,
      version: buildItemVersion({
        patchVersion,
        annotations,
        commentsByThread,
        activeAnnotationKey,
        replyingToThreadId,
        expertise: expertiseByPath.get(file.path),
        expertiseError,
        collapsed,
      }),
    };
  });
}

/**
 * Generated files start collapsed (their header still shows path + stats);
 * `z` inverts the default for the focused file.
 */
export function isFileCollapsed(
  file: ParsedDiffFile,
  collapseOverrides: readonly string[],
): boolean {
  const isOverridden = collapseOverrides.includes(file.canonicalKey);
  return isGeneratedFile(file.path) !== isOverridden;
}

export function findUnrenderableDiffFile(
  files: readonly UnrenderableDiffFile[],
  id: string,
): UnrenderableDiffFile | undefined {
  return files.find(({ file }) => file.id === id);
}

export function groupCommentsByThread(
  comments: readonly Comment[],
): Map<string, Comment[]> {
  const grouped = new Map<string, Comment[]>();
  for (const comment of comments) {
    if (comment.thread_id == null) continue;
    const existing = grouped.get(comment.thread_id);
    if (existing === undefined) {
      grouped.set(comment.thread_id, [comment]);
    } else {
      existing.push(comment);
    }
  }
  return grouped;
}

interface ItemVersionOptions {
  patchVersion: number;
  annotations: readonly DiffLineAnnotation<DiffAnnotationMeta>[];
  commentsByThread: ReadonlyMap<string, readonly Comment[]>;
  activeAnnotationKey: string | null;
  replyingToThreadId: string | null;
  expertise: ExpertiseFileData | undefined;
  expertiseError: string | null;
  collapsed: boolean;
}

function buildItemVersion({
  patchVersion,
  annotations,
  commentsByThread,
  activeAnnotationKey,
  replyingToThreadId,
  expertise,
  expertiseError,
  collapsed,
}: ItemVersionOptions): number {
  const content = [
    serializeExpertise(expertise),
    expertiseError ?? '',
    String(collapsed),
    ...annotations.map((annotation) =>
      serializeAnnotation(annotation, {
        commentsByThread,
        activeAnnotationKey,
        replyingToThreadId,
      }),
    ),
  ].join('|');
  return hashString(`|${content}`, patchVersion);
}

type AnnotationVersionContext = Pick<
  ItemVersionOptions,
  'commentsByThread' | 'activeAnnotationKey' | 'replyingToThreadId'
>;

function serializeAnnotation(
  { side, lineNumber, metadata }: DiffLineAnnotation<DiffAnnotationMeta>,
  context: AnnotationVersionContext,
): string {
  if (metadata === undefined) return `${side}:${lineNumber}`;
  switch (metadata.kind) {
    case 'thread': {
      const { thread } = metadata;
      const comments = context.commentsByThread.get(thread.id) ?? [];
      return [
        side,
        lineNumber,
        thread.id,
        thread.state_known,
        thread.resolved,
        thread.outdated,
        context.activeAnnotationKey === thread.id,
        context.replyingToThreadId === thread.id,
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
    }
    case 'draft':
      return [
        side,
        lineNumber,
        metadata.key,
        metadata.draft.body,
        metadata.isOutdated,
        context.activeAnnotationKey === metadata.key,
      ].join(':');
    case 'composer': {
      const range = cursorRange(metadata.lineComment.cursor);
      return [
        metadata.key,
        metadata.lineComment.composer?.draftId ?? '',
        range.side,
        range.startLine ?? '',
        range.line,
      ].join(':');
    }
    default: {
      const unreachable: never = metadata;
      return unreachable;
    }
  }
}

function serializeExpertise(expertise: ExpertiseFileData | undefined): string {
  if (expertise === undefined) return '';
  return [
    expertise.file_path,
    expertise.your_rank,
    ...expertise.experts.map(serializeExpertiseExpert),
  ].join(':');
}

function serializeExpertiseExpert(expert: ExpertiseExpert): string {
  return [
    expert.author_email,
    expert.author_login,
    expert.surviving_lines,
    expert.total_lines,
    expert.commit_score,
    expert.is_original,
    expert.score,
    expert.dormant,
  ].join(':');
}

function hashString(value: string, initialHash = 0): number {
  let hash = initialHash;
  for (let index = 0; index < value.length; index += 1) {
    hash = (hash * 31 + value.charCodeAt(index)) | 0;
  }
  return hash;
}
