import { CodeView, type CodeViewHandle } from '@pierre/diffs/react';
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import type { AppSettings } from '../../../types/settings';
import { useStore } from '../../hooks/useStore';
import { appStore } from '../../store';
import type { Comment, ExpertiseFileData, ReviewThread } from '../../types';
import { findFileByKey } from '../../utils/diffFileKey';
import { buildOrderedAnnotations } from './diffAnnotations';
import {
  buildCodeViewItems,
  findUnrenderableDiffFile,
  groupCommentsByThread,
  type ParsedCodeViewFile,
  parseCodeViewFiles,
} from './diffCodeViewItems';
import {
  requestSelectedFileScroll,
  scrollToCodeViewFile,
} from './diffCodeViewNavigation';
import { DiffFileExpertise } from './diffFileExpertise';
import { DiffFileSidebar } from './diffFileSidebar';
import { DiffParseFailurePanel } from './diffParseFailurePanel';
import { type ParsedDiffFile, splitUnifiedDiff } from './diffPathUtils';
import { ThreadAnnotationCard } from './diffThreadAnnotation';

function sameIds(left: string[], right: string[]): boolean {
  if (left.length !== right.length) return false;
  return left.every((value, index) => value === right[index]);
}

export interface DiffViewProps {
  diff: string;
  theme: AppSettings['appearance']['theme'];
  findQuery?: string;
  selectedFileKey?: string;
  onFilesResolved?: (canonicalKeys: string[]) => void;
  diffStyle?: AppSettings['diff']['style'];
  threads?: ReviewThread[];
  comments?: Comment[];
  onThreadReply?: (threadId: string, body: string) => void;
  expertiseFiles?: ExpertiseFileData[];
  expertiseError?: string | null;
}

/**
 * CodeView-backed multi-file PR diff. CodeView owns the only virtualized scroll
 * surface; the host stays bounded so its internal container can measure and
 * scroll files correctly.
 */
export function DiffView({
  diff,
  theme,
  findQuery = '',
  selectedFileKey,
  onFilesResolved,
  diffStyle = 'unified',
  threads = [],
  comments = [],
  onThreadReply,
  expertiseFiles = [],
  expertiseError = null,
}: DiffViewProps) {
  const files = useMemo(() => splitUnifiedDiff(diff), [diff]);
  const codeViewFileResult = useMemo(() => parseCodeViewFiles(files), [files]);
  const { parsedFiles: parsedCodeViewFiles, unrenderableFiles } =
    codeViewFileResult;
  const expertiseByPath = useMemo(
    () => new Map(expertiseFiles.map((file) => [file.file_path, file])),
    [expertiseFiles],
  );
  const [activeId, setActiveId] = useState<string | null>(files[0]?.id ?? null);
  const codeViewRef = useRef<CodeViewHandle<ReviewThread>>(null);
  const pendingScrollFileIdRef = useRef<string | null>(null);
  const viewportFileIdRef = useRef<string | null>(null);
  const selectedFileIndex = useStore(
    appStore,
    (state) => state.selectedFileIndex,
  );
  const activeAnnotationKey = useStore(
    appStore,
    (state) => state.activeInlineCommentThreadId,
  );
  const replyingToThreadId = useStore(
    appStore,
    (state) => state.replyingInlineCommentThreadId,
  );

  const orderedAnnotations = useMemo(
    () =>
      buildOrderedAnnotations(
        parsedCodeViewFiles.map(({ file }) => file),
        threads,
      ),
    [parsedCodeViewFiles, threads],
  );
  const commentsByThread = useMemo(
    () => groupCommentsByThread(comments),
    [comments],
  );
  const items = useMemo(
    () =>
      buildCodeViewItems(
        parsedCodeViewFiles,
        threads,
        commentsByThread,
        activeAnnotationKey,
        replyingToThreadId,
        expertiseByPath,
        expertiseError,
      ),
    [
      parsedCodeViewFiles,
      threads,
      commentsByThread,
      activeAnnotationKey,
      replyingToThreadId,
      expertiseByPath,
      expertiseError,
    ],
  );

  useEffect(() => {
    const ids = orderedAnnotations.map((entry) => entry.key);
    appStore.setState((state) => {
      const activeStillValid =
        state.activeInlineCommentThreadId !== null &&
        ids.includes(state.activeInlineCommentThreadId);
      const replyingStillValid =
        state.replyingInlineCommentThreadId !== null &&
        ids.includes(state.replyingInlineCommentThreadId);
      if (
        sameIds(state.inlineCommentThreadIds, ids) &&
        (state.activeInlineCommentThreadId === null || activeStillValid) &&
        (state.replyingInlineCommentThreadId === null || replyingStillValid)
      ) {
        return state;
      }
      return {
        ...state,
        inlineCommentThreadIds: ids,
        activeInlineCommentThreadId: activeStillValid
          ? state.activeInlineCommentThreadId
          : null,
        replyingInlineCommentThreadId: replyingStillValid
          ? state.replyingInlineCommentThreadId
          : null,
      };
    });
  }, [orderedAnnotations]);

  useEffect(() => {
    return () => {
      appStore.setState((state) => ({
        ...state,
        inlineCommentThreadIds: [],
        activeInlineCommentThreadId: null,
        replyingInlineCommentThreadId: null,
      }));
    };
  }, []);

  useEffect(() => {
    if (activeAnnotationKey === null) return;
    const entry = orderedAnnotations.find(
      (annotation) => annotation.key === activeAnnotationKey,
    );
    if (entry === undefined) return;
    scrollToCodeViewFile(codeViewRef.current, entry.fileId, 'nearest');
    codeViewRef.current?.scrollTo({
      type: 'line',
      id: entry.fileId,
      lineNumber: entry.line,
      side: 'additions',
      align: 'nearest',
      behavior: 'instant',
    });
    const animationFrame = requestAnimationFrame(() => {
      document
        .querySelector<HTMLElement>(
          `[data-annotation-key="${CSS.escape(activeAnnotationKey)}"]`,
        )
        ?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
    return () => cancelAnimationFrame(animationFrame);
  }, [activeAnnotationKey, orderedAnnotations]);

  useEffect(() => {
    if (files.length === 0) {
      setActiveId(null);
      return;
    }
    if (activeId === null || !files.some((file) => file.id === activeId)) {
      setActiveId(files[0]?.id ?? null);
    }
  }, [files, activeId]);

  useLayoutEffect(() => {
    onFilesResolved?.(files.map((file) => file.canonicalKey));
  }, [files, onFilesResolved]);

  useEffect(() => {
    if (files.length === 0 || selectedFileKey === undefined) return;
    const matchedFile =
      (selectedFileKey !== ''
        ? findFileByKey(files, selectedFileKey)
        : undefined) ?? files[0];
    if (matchedFile === undefined) return;
    setActiveId(matchedFile.id);
    viewportFileIdRef.current = matchedFile.id;
    requestSelectedFileScroll(
      codeViewRef.current,
      pendingScrollFileIdRef,
      matchedFile.id,
      unrenderableFiles,
    );
    updateSelectedFileIndex(files, matchedFile.id);
    // The active id deliberately stays out of the dependencies: URL changes
    // are the navigation source and scroll-driven updates must not scroll back.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedFileKey, files, unrenderableFiles]);

  useEffect(() => {
    if (files.length === 0) return;
    const clampedIndex = Math.min(
      Math.max(selectedFileIndex, 0),
      files.length - 1,
    );
    if (clampedIndex !== selectedFileIndex) {
      appStore.setState((state) => ({
        ...state,
        selectedFileIndex: clampedIndex,
      }));
      return;
    }
    const file = files[clampedIndex];
    if (file === undefined || file.id === activeId) return;
    setActiveId(file.id);
    viewportFileIdRef.current = file.id;
    requestSelectedFileScroll(
      codeViewRef.current,
      pendingScrollFileIdRef,
      file.id,
      unrenderableFiles,
    );
  }, [selectedFileIndex, files, activeId, unrenderableFiles]);

  const activeUnrenderableFile = findUnrenderableDiffFile(
    unrenderableFiles,
    activeId ?? '',
  );
  const filteredFiles = useMemo(() => {
    const query = findQuery.trim().toLowerCase();
    if (query === '') return files;
    return files.filter((file) => file.path.toLowerCase().includes(query));
  }, [files, findQuery]);

  const handleActiveChange = (id: string): void => {
    setActiveId(id);
    updateSelectedFileIndex(files, id);
  };

  if (files.length === 0) {
    return (
      <div className="w-full h-full flex items-center justify-center text-outline font-mono text-xs uppercase tracking-widest">
        No changes
      </div>
    );
  }

  return (
    <div className="w-full h-full min-h-0 flex overflow-hidden">
      <DiffFileSidebar
        files={filteredFiles}
        activeId={activeId}
        onSelect={(id) => {
          setActiveId(id);
          viewportFileIdRef.current = id;
          requestSelectedFileScroll(
            codeViewRef.current,
            pendingScrollFileIdRef,
            id,
            unrenderableFiles,
          );
          updateSelectedFileIndex(files, id);
        }}
        totalCount={files.length}
      />
      <section className="flex-1 min-w-0 min-h-0 bg-surface-container-lowest overflow-hidden relative">
        {activeUnrenderableFile !== undefined ? (
          <DiffParseFailurePanel file={activeUnrenderableFile} />
        ) : (
          <CodeView
            ref={codeViewRef}
            containerRef={(node) => {
              if (node === null) return;
              node.setAttribute('data-diff-viewport', '');
              const pendingId = pendingScrollFileIdRef.current;
              if (pendingId === null) return;
              requestAnimationFrame(() => {
                if (pendingScrollFileIdRef.current !== pendingId) return;
                scrollToCodeViewFile(codeViewRef.current, pendingId);
                pendingScrollFileIdRef.current = null;
              });
            }}
            className="h-full w-full overflow-y-auto overflow-x-auto"
            disableWorkerPool
            items={items}
            options={{
              theme: `pierre-${theme}`,
              diffStyle,
              lineDiffType: 'word',
              stickyHeaders: true,
              layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
            }}
            onScroll={(scrollTop, viewer) => {
              const visibleFile = findTopVisibleFile(
                parsedCodeViewFiles,
                viewer,
                scrollTop,
              );
              if (visibleFile !== undefined && visibleFile.id !== activeId) {
                viewportFileIdRef.current = visibleFile.id;
                handleActiveChange(visibleFile.id);
              }
            }}
            renderCustomHeader={(item) => {
              const parsedFile = files.find((file) => file.id === item.id);
              if (parsedFile === undefined) return null;
              return (
                <>
                  <div className="bg-surface-container-lowest border-b border-outline-variant/10 px-4 py-2 flex items-center justify-between">
                    <div className="flex items-center gap-2 min-w-0">
                      <span className="text-outline font-mono text-[11px] truncate">
                        {parsedFile.path}
                      </span>
                      <span className="bg-surface-container px-2 py-0.5 text-[10px] text-outline uppercase font-mono shrink-0">
                        {parsedFile.lang}
                      </span>
                    </div>
                    <div className="flex gap-4 font-mono text-[11px] font-bold shrink-0">
                      <span className="text-primary">
                        +{parsedFile.additions}
                      </span>
                      <span className="text-error">
                        -{parsedFile.deletions}
                      </span>
                    </div>
                  </div>
                  <DiffFileExpertise
                    expertise={expertiseByPath.get(parsedFile.path)}
                    error={expertiseError}
                  />
                </>
              );
            }}
            renderAnnotation={(annotation) => {
              const thread = annotation.metadata;
              if (thread === undefined) return null;
              const threadComments = commentsByThread.get(thread.id) ?? [];
              return (
                <ThreadAnnotationCard
                  thread={thread}
                  comments={threadComments}
                  isActive={thread.id === activeAnnotationKey}
                  isReplying={thread.id === replyingToThreadId}
                  onActivate={() => {
                    appStore.setState((state) => ({
                      ...state,
                      activeInlineCommentThreadId: thread.id,
                    }));
                  }}
                  onReplyClose={() => {
                    appStore.setState((state) => ({
                      ...state,
                      replyingInlineCommentThreadId: null,
                    }));
                  }}
                  {...(onThreadReply !== undefined ? { onThreadReply } : {})}
                />
              );
            }}
          />
        )}
      </section>
    </div>
  );
}

function updateSelectedFileIndex(
  files: readonly ParsedDiffFile[],
  id: string,
): void {
  const index = files.findIndex((file) => file.id === id);
  if (index >= 0 && appStore.state.selectedFileIndex !== index) {
    appStore.setState((state) => ({ ...state, selectedFileIndex: index }));
  }
}

function findTopVisibleFile(
  files: readonly ParsedCodeViewFile[],
  viewer: ReturnType<CodeViewHandle<ReviewThread>['getInstance']>,
  scrollTop: number,
): ParsedDiffFile | undefined {
  if (viewer === undefined) return undefined;
  let visibleFile: ParsedDiffFile | undefined;
  for (const { file } of files) {
    const top = viewer.getTopForItem(file.id);
    if (top === undefined || top > scrollTop + 1) break;
    visibleFile = file;
  }
  return visibleFile ?? files[0]?.file;
}
