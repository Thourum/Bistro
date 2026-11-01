import { useMemo } from 'react';
import { useStore } from '../../hooks/useStore';
import { appStore } from '../../store';
import type {
  Comment,
  ExpertiseFileData,
  ReviewDraft,
  ReviewThread,
} from '../../types';
import { hideWhitespaceOnlyHunks } from '../../utils/whitespaceDiff';
import { buildOrderedAnnotations } from './diffAnnotations';
import {
  buildCodeViewItems,
  groupCommentsByThread,
  parseCodeViewFiles,
} from './diffCodeViewItems';
import { type ParsedDiffFile, splitUnifiedDiff } from './diffPathUtils';

interface DiffViewDataOptions {
  diff: string;
  threads: readonly ReviewThread[];
  drafts: readonly ReviewDraft[];
  headSha: string;
  comments: readonly Comment[];
  expertiseFiles: readonly ExpertiseFileData[];
  expertiseError: string | null;
  oversizedPatchBytes: ReadonlyMap<string, number>;
}

export function useDiffViewData({
  diff,
  threads,
  drafts,
  headSha,
  comments,
  expertiseFiles,
  expertiseError,
  oversizedPatchBytes,
}: DiffViewDataOptions) {
  const hideWhitespaceHunks = useStore(
    appStore,
    (state) => state.hideWhitespaceHunks,
  );
  const collapseOverrides = useStore(
    appStore,
    (state) => state.diffCollapseOverrides,
  );
  const activeAnnotationKey = useStore(
    appStore,
    (state) => state.activeInlineCommentThreadId,
  );
  const replyingToThreadId = useStore(
    appStore,
    (state) => state.replyingInlineCommentThreadId,
  );
  const lineComment = useStore(appStore, (state) => state.lineComment);
  const allFiles = useMemo(() => splitUnifiedDiff(diff), [diff]);
  // Hiding whitespace hunks rewrites patches without changing file identity.
  const { files, hiddenWhitespaceByFileId } = useMemo(
    () => filterWhitespaceHunks(allFiles, hideWhitespaceHunks),
    [allFiles, hideWhitespaceHunks],
  );
  const { parsedFiles, unrenderableFiles } = useMemo(
    () => parseCodeViewFiles(files, oversizedPatchBytes),
    [files, oversizedPatchBytes],
  );
  const expertiseByPath = useMemo(
    () => new Map(expertiseFiles.map((file) => [file.file_path, file])),
    [expertiseFiles],
  );
  const commentsByThread = useMemo(
    () => groupCommentsByThread(comments),
    [comments],
  );
  const orderedAnnotations = useMemo(
    () => buildOrderedAnnotations(parsedFiles, threads, drafts),
    [parsedFiles, threads, drafts],
  );
  const items = useMemo(
    () =>
      buildCodeViewItems({
        parsedCodeViewFiles: parsedFiles,
        threads,
        drafts,
        headSha,
        lineComment,
        commentsByThread,
        activeAnnotationKey,
        replyingToThreadId,
        expertiseByPath,
        expertiseError,
        collapseOverrides,
      }),
    [
      parsedFiles,
      threads,
      drafts,
      headSha,
      lineComment,
      commentsByThread,
      activeAnnotationKey,
      replyingToThreadId,
      expertiseByPath,
      expertiseError,
      collapseOverrides,
    ],
  );

  return {
    files,
    hiddenWhitespaceByFileId,
    parsedFiles,
    unrenderableFiles,
    expertiseByPath,
    commentsByThread,
    orderedAnnotations,
    items,
    activeAnnotationKey,
    replyingToThreadId,
  };
}

interface WhitespaceFilterResult {
  files: ParsedDiffFile[];
  /** File id → number of `+`/`-` lines hidden in that file. */
  hiddenWhitespaceByFileId: ReadonlyMap<string, number>;
}

function filterWhitespaceHunks(
  files: readonly ParsedDiffFile[],
  enabled: boolean,
): WhitespaceFilterResult {
  if (!enabled)
    return { files: [...files], hiddenWhitespaceByFileId: new Map() };
  const hidden = new Map<string, number>();
  const filtered = files.map((file) => {
    const { patch, hiddenLineCount } = hideWhitespaceOnlyHunks(file.patch);
    if (hiddenLineCount === 0) return file;
    hidden.set(file.id, hiddenLineCount);
    return { ...file, patch };
  });
  return { files: filtered, hiddenWhitespaceByFileId: hidden };
}
