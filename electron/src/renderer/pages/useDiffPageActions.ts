import { Fault, ResultAsync } from '@thourum/fault';
import { useCallback, useEffect, useRef } from 'react';
import { hasCurrentInlineAnchor } from '../components/diff/diffAnnotations';
import { splitUnifiedDiff } from '../components/diff/diffPathUtils';
import { addToast } from '../components/ToastContainer';
import { useMarkFileViewed } from '../hooks/useMarkFileViewed';
import { useOpenInEditor } from '../hooks/useOpenInEditor';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import type { UIState } from '../store/ui';
import type { DiffFile, PR, ReviewThread } from '../types';
import { firstHunkAdditionLine } from '../utils/firstHunkLine';

interface DiffPageActionsOptions {
  pr: PR | undefined;
  diff: string;
  currentFile: DiffFile | undefined;
  threads: ReviewThread[];
  confirmingAction: UIState['confirmingAction'];
  setFiles: React.Dispatch<React.SetStateAction<DiffFile[]>>;
}

/** Handles diff page keyboard intents and editor opening without changing fetches. */
export function useDiffPageActions({
  pr,
  diff,
  currentFile,
  threads,
  confirmingAction,
  setFiles,
}: DiffPageActionsOptions): ReturnType<typeof useOpenInEditor> {
  const diffStyle = useStore(settingsStore, (s) => s.diff.style);
  const { markViewed } = useMarkFileViewed();
  const openInEditor = useOpenInEditor();
  const { state, requestOpen } = openInEditor;
  const markInFlight = useRef(false);

  const doMarkViewed = useCallback(() => {
    if (pr === undefined || currentFile === undefined || markInFlight.current)
      return;
    markInFlight.current = true;
    const { path, content_hash } = currentFile;
    // The markViewed hook already handles transport rejection and returns null.
    void ResultAsync.fromPromise(
      markViewed(pr.id, path, content_hash),
      (cause) =>
        new Fault(cause instanceof Error ? cause : 'Could not mark file viewed')
          .withTag('MARK_VIEWED_ERROR')
          .withCause(cause),
    ).match(
      (ack) => {
        if (ack?.ok) {
          setFiles((prev) =>
            prev.map((file) =>
              file.path === path ? { ...file, viewed: true } : file,
            ),
          );
        } else {
          addToast({
            type: 'warning',
            message: ack?.reason
              ? `Could not mark viewed: ${ack.reason}`
              : 'Could not mark viewed: network or communication error',
            dismissible: true,
          });
        }
        markInFlight.current = false;
      },
      () => {
        addToast({
          type: 'warning',
          message: 'Could not mark viewed: network or communication error',
          dismissible: true,
        });
        markInFlight.current = false;
      },
    );
  }, [pr, currentFile, markViewed, setFiles]);

  useEffect(() => {
    if (confirmingAction !== 'mark-viewed') return;
    doMarkViewed();
    appStore.setState((s) => ({ ...s, confirmingAction: null }));
  }, [confirmingAction, doMarkViewed]);

  // The selected thread's line wins; otherwise open on the first new-side line.
  useEffect(() => {
    if (confirmingAction !== 'open-in-editor') return;
    appStore.setState((s) => ({ ...s, confirmingAction: null }));
    if (pr === undefined || currentFile === undefined) {
      addToast({
        type: 'warning',
        message: 'Could not open in editor: no file focused',
        dismissible: true,
      });
      return;
    }
    const activeThread = threads.find(
      (thread) =>
        thread.id === appStore.state.activeInlineCommentThreadId &&
        thread.file_path === currentFile.path &&
        hasCurrentInlineAnchor(thread),
    );
    const filePatch = splitUnifiedDiff(diff).find(
      (file) => file.path === currentFile.path,
    )?.patch;
    const line =
      activeThread?.line ??
      (filePatch !== undefined ? firstHunkAdditionLine(filePatch) : 1);
    void requestOpen(pr.id, currentFile.path, line);
  }, [confirmingAction, pr, currentFile, threads, diff, requestOpen]);

  useEffect(() => {
    if (state.error === null) return;
    addToast({
      type: 'warning',
      message: `Could not open in editor: ${state.error}`,
      dismissible: true,
    });
  }, [state.error]);

  useEffect(() => {
    if (confirmingAction !== 'toggle-diff-style') return;
    const nextStyle = diffStyle === 'split' ? 'unified' : 'split';
    appStore.setState((s) => ({ ...s, confirmingAction: null }));
    updateSettings({ diff: { style: nextStyle } }).match(
      () => undefined,
      (error) =>
        addToast({ type: 'error', message: error.message, dismissible: true }),
    );
  }, [confirmingAction, diffStyle]);

  useEffect(() => {
    return () => {
      appStore.setState((s) =>
        s.confirmingAction === 'since-last-review'
          ? { ...s, confirmingAction: null }
          : s,
      );
    };
  }, []);

  return openInEditor;
}
