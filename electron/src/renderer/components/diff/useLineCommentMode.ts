import type { CodeViewLineSelection } from '@pierre/diffs';
import type { CodeViewHandle } from '@pierre/diffs/react';
import { useEffect, useMemo, useRef } from 'react';
import { useStore } from '../../hooks/useStore';
import { useOverlays } from '../../overlays/OverlayContext';
import { appStore } from '../../store';
import {
  cursorRange,
  type DiffLayout,
  initialCursor,
  type LineCommentState,
  listCommentableLines,
  toAnnotationSide,
} from '../../utils/lineCommentCursor';
import { addToast } from '../ToastContainer';
import { type DiffAnnotationMeta, lineExistsOnSide } from './diffAnnotations';
import type { ParsedCodeViewFile } from './diffCodeViewItems';

const OPEN_LINE_COMMENT_EVENT = 'bistro:diff:line-comment';

interface LineCommentModeOptions {
  prId: string | undefined;
  headSha: string | undefined;
  parsedFiles: readonly ParsedCodeViewFile[];
  activeFileId: string | null;
  layout: DiffLayout;
  codeViewRef: React.RefObject<CodeViewHandle<
    DiffAnnotationMeta,
    unknown
  > | null>;
}

function clearLineComment(): void {
  appStore.setState((state) =>
    state.lineComment === null ? state : { ...state, lineComment: null },
  );
}

/**
 * Opens comment mode on `c`, keeps the cursor scrolled into view, and mirrors
 * it into CodeView's controlled line selection.
 */
export function useLineCommentMode({
  prId,
  headSha,
  parsedFiles,
  activeFileId,
  layout,
  codeViewRef,
}: LineCommentModeOptions): CodeViewLineSelection | null {
  const { activeOverlay, closeOverlay, openLineComment } = useOverlays();
  const lineComment = useStore(appStore, (state) => state.lineComment);

  useEffect(() => {
    const open = (): void => {
      if (prId === undefined || headSha === undefined) return;
      const parsed =
        parsedFiles.find(({ file }) => file.id === activeFileId) ??
        parsedFiles[0];
      if (parsed === undefined) return;
      const lines = listCommentableLines(parsed.fileDiff.hunks, layout);
      const cursor = initialCursor(lines);
      if (cursor === null) {
        addToast({
          type: 'warning',
          message: 'No commentable lines in this file',
          dismissible: true,
        });
        return;
      }
      const state: LineCommentState = {
        prId,
        fileId: parsed.file.id,
        path: parsed.file.path,
        headSha,
        layout,
        lines,
        cursor,
        composer: null,
      };
      appStore.setState((current) => ({ ...current, lineComment: state }));
      openLineComment();
    };
    document.addEventListener(OPEN_LINE_COMMENT_EVENT, open);
    return () => document.removeEventListener(OPEN_LINE_COMMENT_EVENT, open);
  }, [prId, headSha, parsedFiles, activeFileId, layout, openLineComment]);

  // Any overlay change (Esc, save, another overlay) ends comment mode.
  useEffect(() => {
    if (activeOverlay !== 'lineComment') clearLineComment();
  }, [activeOverlay]);

  // Unmount (e.g. leaving the diff by mouse) must not leave the overlay open.
  const activeOverlayRef = useRef(activeOverlay);
  activeOverlayRef.current = activeOverlay;
  useEffect(
    () => () => {
      if (activeOverlayRef.current === 'lineComment') closeOverlay();
      clearLineComment();
    },
    [closeOverlay],
  );

  // The commentable-line list depends on layout, so a layout switch ends comment mode.
  const previousLayoutRef = useRef(layout);
  useEffect(() => {
    if (previousLayoutRef.current === layout) return;
    previousLayoutRef.current = layout;
    if (activeOverlay === 'lineComment') closeOverlay();
  }, [layout, activeOverlay, closeOverlay]);

  useEffect(() => {
    if (lineComment === null) return;
    // Follow the moving end so extending upward keeps it in view.
    const { head } = lineComment.cursor;
    const side = toAnnotationSide(lineComment.cursor.side);
    const parsed = parsedFiles.find(
      ({ file }) => file.id === lineComment.fileId,
    );
    if (parsed === undefined || !lineExistsOnSide(parsed.fileDiff, head, side))
      return;
    codeViewRef.current?.scrollTo({
      type: 'line',
      id: lineComment.fileId,
      lineNumber: head,
      side,
      align: 'nearest',
      behavior: 'instant',
    });
  }, [lineComment, parsedFiles, codeViewRef]);

  return useMemo(() => {
    if (lineComment === null) return null;
    const range = cursorRange(lineComment.cursor);
    const side = toAnnotationSide(range.side);
    return {
      id: lineComment.fileId,
      range: {
        start: range.startLine ?? range.line,
        end: range.line,
        side,
        endSide: side,
      },
    };
  }, [lineComment]);
}
