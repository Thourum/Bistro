import type { CodeViewItem } from '@pierre/diffs';
import {
  CodeView,
  type CodeViewHandle,
  useWorkerPool,
  WorkerPoolContextProvider,
  type WorkerPoolOptions,
} from '@pierre/diffs/react';
import DiffsHighlighterWorker from '@pierre/diffs/worker/worker.js?worker';
import { Fault, ResultAsync } from '@thourum/fault';
import { useEffect, useMemo } from 'react';
import type { AppSettings } from '../../../types/settings';
import { useStore } from '../../hooks/useStore';
import { appStore } from '../../store';
import type {
  Comment,
  ExpertiseFileData,
  ReviewThread,
  TriageFileStatus,
} from '../../types';
import { createLogger } from '../../utils/logger';
import { openUrlInBrowser } from '../../utils/openUrl';
import { hideWhitespaceOnlyHunks } from '../../utils/whitespaceDiff';
import { buildOrderedAnnotations } from './diffAnnotations';
import {
  buildCodeViewItems,
  findUnrenderableDiffFile,
  groupCommentsByThread,
  type ParsedCodeViewFile,
  parseCodeViewFiles,
} from './diffCodeViewItems';
import { scrollToCodeViewFile } from './diffCodeViewNavigation';
import { DiffFileExpertise } from './diffFileExpertise';
import { DiffFileSidebar } from './diffFileSidebar';
import { DiffParseFailurePanel } from './diffParseFailurePanel';
import { type ParsedDiffFile, splitUnifiedDiff } from './diffPathUtils';
import { ThreadAnnotationCard } from './diffThreadAnnotation';
import {
  useDiffFileNavigation,
  useInlineDiffAnnotations,
} from './diffViewNavigation';

const OPEN_FILE_BROWSER_EVENT = 'bistro:diff:open-file-browser';
const log = createLogger('DiffView');
// Lines longer than this render unhighlighted, so a minified bundle can't
// pin a worker. 1,000 chars covers hand-written code with margin.
const TOKENIZE_MAX_LINE_LENGTH = 1000;
const LINE_DIFF_TYPE = 'word';

const WORKER_POOL_OPTIONS: WorkerPoolOptions = {
  workerFactory: () => new DiffsHighlighterWorker(),
  // Pool stops workers and falls back to main-thread highlighting on failure.
  onWorkerError: (event) => {
    event.preventDefault();
    log.error(
      { error: event instanceof ErrorEvent ? event.message : event.type },
      'diff highlighter worker failed',
    );
  },
};

const EMPTY_VIEWED_PATHS: ReadonlySet<string> = new Set();
const EMPTY_OVERSIZED: ReadonlyMap<string, number> = new Map();
const EMPTY_THREADS: readonly ReviewThread[] = [];
const EMPTY_COMMENTS: readonly Comment[] = [];
const EMPTY_EXPERTISE_FILES: readonly ExpertiseFileData[] = [];

export interface DiffViewProps {
  diff: string;
  theme: AppSettings['appearance']['theme'];
  findQuery?: string;
  selectedFileKey?: string;
  onFilesResolved?: (canonicalKeys: string[]) => void;
  /** Interdiff disables this so its one-file view preserves the triage selection. */
  syncFileSelection?: boolean;
  diffStyle?: AppSettings['diff']['style'];
  threads?: readonly ReviewThread[];
  comments?: readonly Comment[];
  onThreadReply?: (threadId: string, body: string) => void;
  expertiseFiles?: readonly ExpertiseFileData[];
  expertiseError?: string | null;
  /** Base PR URL on the provider (no `/files` suffix). */
  prBrowserUrl?: string | null;
  /** Paths marked viewed (`v`) for the current user. */
  viewedPaths?: ReadonlySet<string>;
  /** Since-last-review status per path, shown as sidebar badges. */
  triageStatuses?: ReadonlyMap<string, TriageFileStatus>;
  /** Engine-capped files (no stored patch), keyed by path → original patch bytes. */
  oversizedPatchBytes?: ReadonlyMap<string, number>;
}

/** CodeView owns the only virtualized scroll surface for the PR diff. */
export function DiffView({
  diff,
  theme,
  findQuery = '',
  selectedFileKey,
  onFilesResolved,
  syncFileSelection = true,
  diffStyle = 'unified',
  threads = EMPTY_THREADS,
  comments = EMPTY_COMMENTS,
  onThreadReply,
  expertiseFiles = EMPTY_EXPERTISE_FILES,
  expertiseError = null,
  prBrowserUrl = null,
  viewedPaths = EMPTY_VIEWED_PATHS,
  triageStatuses,
  oversizedPatchBytes = EMPTY_OVERSIZED,
}: DiffViewProps): React.ReactElement {
  const {
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
  } = useDiffViewData({
    diff,
    threads,
    comments,
    expertiseFiles,
    expertiseError,
    oversizedPatchBytes,
  });
  const {
    activeId,
    codeViewRef,
    pendingScrollFileIdRef,
    selectFile,
    activateVisibleFile,
  } = useDiffFileNavigation({
    files,
    unrenderableFiles,
    selectedFileKey,
    onFilesResolved,
    syncFileSelection,
  });
  useInlineDiffAnnotations({
    orderedAnnotations,
    parsedCodeViewFiles: parsedFiles,
    codeViewRef,
  });

  const openFilesInBrowser = useFilesBrowserShortcut(prBrowserUrl);
  const filteredFiles = useMemo(() => {
    const query = findQuery.trim().toLowerCase();
    return query === ''
      ? files
      : files.filter((file) => file.path.toLowerCase().includes(query));
  }, [files, findQuery]);

  if (files.length === 0) {
    return (
      <div className="w-full h-full flex items-center justify-center text-outline font-mono text-xs uppercase tracking-widest">
        No changes
      </div>
    );
  }

  const activeUnrenderableFile = findUnrenderableDiffFile(
    unrenderableFiles,
    activeId ?? '',
  );
  return (
    <WorkerPoolContextProvider
      poolOptions={WORKER_POOL_OPTIONS}
      highlighterOptions={{
        theme: `pierre-${theme}`,
        lineDiffType: LINE_DIFF_TYPE,
        tokenizeMaxLineLength: TOKENIZE_MAX_LINE_LENGTH,
        // Oniguruma (wasm) over the default JS regex engine: on Marlin #28644
        // it cut large-file highlight from ~440 ms to ~100 ms and worker heaps
        // from ~120 MB to <50 MB, which is what lets 8 workers fit in memory.
        preferredHighlighter: 'shiki-wasm',
      }}
    >
      <div className="w-full h-full min-h-0 flex overflow-hidden">
        <DiffFileSidebar
          files={filteredFiles}
          activeId={activeId}
          onSelect={selectFile}
          totalCount={files.length}
          viewedPaths={viewedPaths}
          {...(triageStatuses !== undefined ? { triageStatuses } : {})}
        />
        <section className="flex-1 min-w-0 min-h-0 bg-surface-container-lowest overflow-hidden relative">
          {activeUnrenderableFile !== undefined ? (
            <DiffParseFailurePanel
              file={activeUnrenderableFile}
              {...(openFilesInBrowser !== undefined
                ? { onOpenInBrowser: openFilesInBrowser }
                : {})}
            />
          ) : (
            <DiffCodeView
              files={files}
              parsedFiles={parsedFiles}
              items={items}
              codeViewRef={codeViewRef}
              pendingScrollFileIdRef={pendingScrollFileIdRef}
              activeId={activeId}
              activateVisibleFile={activateVisibleFile}
              hiddenWhitespaceByFileId={hiddenWhitespaceByFileId}
              expertiseByPath={expertiseByPath}
              expertiseError={expertiseError}
              commentsByThread={commentsByThread}
              activeAnnotationKey={activeAnnotationKey}
              replyingToThreadId={replyingToThreadId}
              onThreadReply={onThreadReply}
              theme={theme}
              diffStyle={diffStyle}
            />
          )}
        </section>
      </div>
    </WorkerPoolContextProvider>
  );
}

function useFilesBrowserShortcut(
  prBrowserUrl: string | null,
): (() => void) | undefined {
  const filesBrowserUrl =
    prBrowserUrl === null || prBrowserUrl === ''
      ? null
      : `${prBrowserUrl.replace(/\/+$/, '')}/files`;
  useEffect(() => {
    if (filesBrowserUrl === null) return;
    const handler = (): void => openFilesInBrowser(filesBrowserUrl);
    document.addEventListener(OPEN_FILE_BROWSER_EVENT, handler);
    return () => document.removeEventListener(OPEN_FILE_BROWSER_EVENT, handler);
  }, [filesBrowserUrl]);
  return filesBrowserUrl === null
    ? undefined
    : () => openFilesInBrowser(filesBrowserUrl);
}

function openFilesInBrowser(url: string): void {
  void openUrlInBrowser(url).orInspect((error) =>
    log.error({ error: error.message }, 'failed to open diff in browser'),
  );
}

interface DiffViewDataOptions {
  diff: string;
  threads: readonly ReviewThread[];
  comments: readonly Comment[];
  expertiseFiles: readonly ExpertiseFileData[];
  expertiseError: string | null;
  oversizedPatchBytes: ReadonlyMap<string, number>;
}

function useDiffViewData({
  diff,
  threads,
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
    () =>
      buildOrderedAnnotations(
        parsedFiles.map(({ file }) => file),
        threads,
      ),
    [parsedFiles, threads],
  );
  const items = useMemo(
    () =>
      buildCodeViewItems({
        parsedCodeViewFiles: parsedFiles,
        threads,
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

interface DiffCodeViewProps {
  files: readonly ParsedDiffFile[];
  parsedFiles: readonly ParsedCodeViewFile[];
  items: readonly CodeViewItem<ReviewThread>[];
  codeViewRef: React.RefObject<CodeViewHandle<ReviewThread, unknown> | null>;
  pendingScrollFileIdRef: React.RefObject<string | null>;
  activeId: string | null;
  activateVisibleFile: (id: string) => void;
  hiddenWhitespaceByFileId: ReadonlyMap<string, number>;
  expertiseByPath: ReadonlyMap<string, ExpertiseFileData>;
  expertiseError: string | null;
  commentsByThread: ReadonlyMap<string, readonly Comment[]>;
  activeAnnotationKey: string | null;
  replyingToThreadId: string | null;
  onThreadReply: ((threadId: string, body: string) => void) | undefined;
  theme: DiffViewProps['theme'];
  diffStyle: NonNullable<DiffViewProps['diffStyle']>;
}

interface DiffFileHeaderProps {
  file: ParsedDiffFile;
  hiddenWhitespace: number | undefined;
  expertise: ExpertiseFileData | undefined;
  expertiseError: string | null;
}

function DiffCodeView({
  files,
  parsedFiles,
  items,
  codeViewRef,
  pendingScrollFileIdRef,
  activeId,
  activateVisibleFile,
  hiddenWhitespaceByFileId,
  expertiseByPath,
  expertiseError,
  commentsByThread,
  activeAnnotationKey,
  replyingToThreadId,
  onThreadReply,
  theme,
  diffStyle,
}: DiffCodeViewProps): React.ReactElement {
  const workerPool = useWorkerPool();
  // The pool is a singleton: highlighterOptions apply only at creation, so
  // later theme switches must be pushed to the workers.
  useEffect(() => {
    if (workerPool === undefined) return;
    void ResultAsync.fromPromise(
      workerPool.setRenderOptions({ theme: `pierre-${theme}` }),
      (cause) =>
        new Fault(cause instanceof Error ? cause.message : String(cause))
          .withTag('DIFF_THEME_ERROR')
          .withCause(cause),
    ).orInspect((error) =>
      log.error({ error: error.message }, 'failed to switch diff theme'),
    );
  }, [workerPool, theme]);
  return (
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
      items={items}
      options={{
        theme: `pierre-${theme}`,
        diffStyle,
        lineDiffType: LINE_DIFF_TYPE,
        tokenizeMaxLineLength: TOKENIZE_MAX_LINE_LENGTH,
        stickyHeaders: true,
        layout: { paddingTop: 0, paddingBottom: 0, gap: 0 },
      }}
      onScroll={(scrollTop, viewer) => {
        const visibleFile = findTopVisibleFile(parsedFiles, viewer, scrollTop);
        if (visibleFile !== undefined && visibleFile.id !== activeId)
          activateVisibleFile(visibleFile.id);
      }}
      renderCustomHeader={(item) => {
        const file = files.find((entry) => entry.id === item.id);
        if (file === undefined) return null;
        return (
          <DiffFileHeader
            file={file}
            hiddenWhitespace={hiddenWhitespaceByFileId.get(item.id)}
            expertise={expertiseByPath.get(file.path)}
            expertiseError={expertiseError}
          />
        );
      }}
      renderAnnotation={(annotation) => {
        const thread = annotation.metadata;
        if (thread === undefined) return null;
        return (
          <DiffAnnotation
            thread={thread}
            comments={commentsByThread.get(thread.id) ?? []}
            isActive={thread.id === activeAnnotationKey}
            isReplying={thread.id === replyingToThreadId}
            onThreadReply={onThreadReply}
          />
        );
      }}
    />
  );
}

interface DiffAnnotationProps {
  thread: ReviewThread;
  comments: readonly Comment[];
  isActive: boolean;
  isReplying: boolean;
  onThreadReply: ((threadId: string, body: string) => void) | undefined;
}

function DiffAnnotation({
  thread,
  comments,
  isActive,
  isReplying,
  onThreadReply,
}: DiffAnnotationProps): React.ReactElement {
  return (
    <ThreadAnnotationCard
      thread={thread}
      comments={comments}
      isActive={isActive}
      isReplying={isReplying}
      onActivate={() =>
        appStore.setState((state) => ({
          ...state,
          activeInlineCommentThreadId: thread.id,
        }))
      }
      onReplyClose={() =>
        appStore.setState((state) => ({
          ...state,
          replyingInlineCommentThreadId: null,
        }))
      }
      {...(onThreadReply !== undefined ? { onThreadReply } : {})}
    />
  );
}

function DiffFileHeader({
  file,
  hiddenWhitespace,
  expertise,
  expertiseError,
}: DiffFileHeaderProps): React.ReactElement {
  return (
    <>
      <div className="bg-surface-container-lowest border-b border-outline-variant/10 px-4 py-2 flex items-center justify-between">
        <div className="flex items-center gap-2 min-w-0">
          <span className="text-outline font-mono text-[11px] truncate">
            {file.path}
          </span>
          <span className="bg-surface-container px-2 py-0.5 text-[10px] text-outline uppercase font-mono shrink-0">
            {file.lang}
          </span>
        </div>
        <div className="flex items-center gap-4 font-mono text-[11px] shrink-0">
          {hiddenWhitespace !== undefined && (
            <span className="text-outline font-normal normal-case">
              {hiddenWhitespace} whitespace-only lines hidden
            </span>
          )}
          <span className="text-primary font-bold">+{file.additions}</span>
          <span className="text-error font-bold">-{file.deletions}</span>
        </div>
      </div>
      <DiffFileExpertise expertise={expertise} error={expertiseError} />
    </>
  );
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

function findTopVisibleFile(
  files: readonly ParsedCodeViewFile[],
  viewer: ReturnType<CodeViewHandle<ReviewThread, unknown>['getInstance']>,
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
