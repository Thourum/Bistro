import type { CodeViewHandle } from '@pierre/diffs/react';
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useStore } from '../../hooks/useStore';
import { appStore } from '../../store';
import type { ReviewThread } from '../../types';
import { findFileByKey } from '../../utils/diffFileKey';
import { type AnnotationEntry, lineExistsInAdditions } from './diffAnnotations';
import type {
  ParsedCodeViewFile,
  UnrenderableDiffFile,
} from './diffCodeViewItems';
import {
  requestSelectedFileScroll,
  scrollToCodeViewFile,
} from './diffCodeViewNavigation';
import type { ParsedDiffFile } from './diffPathUtils';

function sameIds(left: readonly string[], right: readonly string[]): boolean {
  if (left.length !== right.length) return false;
  return left.every((value, index) => value === right[index]);
}

export function useInlineDiffAnnotations({
  orderedAnnotations,
  parsedCodeViewFiles,
  codeViewRef,
}: {
  orderedAnnotations: readonly AnnotationEntry[];
  parsedCodeViewFiles: readonly ParsedCodeViewFile[];
  codeViewRef: React.RefObject<CodeViewHandle<ReviewThread, unknown> | null>;
}): void {
  const activeAnnotationKey = useStore(
    appStore,
    (state) => state.activeInlineCommentThreadId,
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
    // A stale or deletion-side anchor may not exist in the locally parsed hunk;
    // CodeView.scrollTo no-ops then, while the DOM fallback still finds the card.
    const parsedFile = parsedCodeViewFiles.find(
      ({ file }) => file.id === entry.fileId,
    );
    if (
      parsedFile !== undefined &&
      lineExistsInAdditions(parsedFile.fileDiff, entry.line)
    ) {
      codeViewRef.current?.scrollTo({
        type: 'line',
        id: entry.fileId,
        lineNumber: entry.line,
        side: 'additions',
        align: 'nearest',
        behavior: 'instant',
      });
    }
    const animationFrame = requestAnimationFrame(() => {
      document
        .querySelector<HTMLElement>(
          `[data-annotation-key="${CSS.escape(activeAnnotationKey)}"]`,
        )
        ?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    });
    return () => cancelAnimationFrame(animationFrame);
  }, [
    activeAnnotationKey,
    orderedAnnotations,
    parsedCodeViewFiles,
    codeViewRef,
  ]);
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

interface FileNavigationOptions {
  files: ParsedDiffFile[];
  unrenderableFiles: readonly UnrenderableDiffFile[];
  selectedFileKey: string | undefined;
  onFilesResolved: ((canonicalKeys: string[]) => void) | undefined;
  syncFileSelection: boolean;
}

export function useDiffFileNavigation({
  files,
  unrenderableFiles,
  selectedFileKey,
  onFilesResolved,
  syncFileSelection,
}: FileNavigationOptions) {
  const [activeId, setActiveId] = useState<string | null>(files[0]?.id ?? null);
  const codeViewRef = useRef<CodeViewHandle<ReviewThread, unknown>>(null);
  const pendingScrollFileIdRef = useRef<string | null>(null);
  const selectedFileIndex = useStore(
    appStore,
    (state) => state.selectedFileIndex,
  );

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
    if (!syncFileSelection) return;
    onFilesResolved?.(files.map((file) => file.canonicalKey));
  }, [files, onFilesResolved, syncFileSelection]);

  useEffect(() => {
    if (files.length === 0 || selectedFileKey === undefined) return;
    const matchedFile =
      (selectedFileKey !== ''
        ? findFileByKey(files, selectedFileKey)
        : undefined) ?? files[0];
    if (matchedFile === undefined) return;
    setActiveId(matchedFile.id);
    requestSelectedFileScroll(
      codeViewRef.current,
      pendingScrollFileIdRef,
      matchedFile.id,
      unrenderableFiles,
    );
    if (syncFileSelection) updateSelectedFileIndex(files, matchedFile.id);
    // The URL, not scroll-driven activeId updates, drives this navigation.
  }, [selectedFileKey, files, unrenderableFiles, syncFileSelection]);

  useEffect(() => {
    if (!syncFileSelection || files.length === 0) return;
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
    requestSelectedFileScroll(
      codeViewRef.current,
      pendingScrollFileIdRef,
      file.id,
      unrenderableFiles,
    );
  }, [
    selectedFileIndex,
    files,
    activeId,
    unrenderableFiles,
    syncFileSelection,
  ]);

  const selectFile = (id: string): void => {
    setActiveId(id);
    requestSelectedFileScroll(
      codeViewRef.current,
      pendingScrollFileIdRef,
      id,
      unrenderableFiles,
    );
    if (syncFileSelection) updateSelectedFileIndex(files, id);
  };

  const activateVisibleFile = (id: string): void => {
    setActiveId(id);
    if (syncFileSelection) updateSelectedFileIndex(files, id);
  };

  return {
    activeId,
    codeViewRef,
    pendingScrollFileIdRef,
    selectFile,
    activateVisibleFile,
  };
}
