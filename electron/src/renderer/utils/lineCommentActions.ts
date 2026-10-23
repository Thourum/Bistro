import type { KeyActionContext } from './keybindingActions';
import {
  extendCursor,
  type LineCommentState,
  moveCursor,
  switchSide,
} from './lineCommentCursor';

/** Cursor keys only steer while the composer is closed. */
function updateLineComment(
  context: KeyActionContext,
  update: (state: LineCommentState) => LineCommentState,
): void {
  context.store.setState((state) =>
    state.lineComment === null || state.lineComment.composer !== null
      ? state
      : { ...state, lineComment: update(state.lineComment) },
  );
}

/** Opens an empty composer; a no-op while one is already open (keeps its draftId). */
function composeLineComment(context: KeyActionContext): void {
  context.store.setState((state) =>
    state.lineComment === null || state.lineComment.composer !== null
      ? state
      : {
          ...state,
          lineComment: {
            ...state.lineComment,
            composer: { draftId: null, initialBody: '' },
          },
        },
  );
}

export const LINE_COMMENT_ACTIONS: Readonly<
  Record<string, (context: KeyActionContext) => void>
> = {
  'lineComment.down': (context) =>
    updateLineComment(context, (lc) => ({
      ...lc,
      cursor: moveCursor(lc.lines, lc.cursor, lc.layout, 1),
    })),
  'lineComment.up': (context) =>
    updateLineComment(context, (lc) => ({
      ...lc,
      cursor: moveCursor(lc.lines, lc.cursor, lc.layout, -1),
    })),
  'lineComment.extendDown': (context) =>
    updateLineComment(context, (lc) => ({
      ...lc,
      cursor: extendCursor(lc.lines, lc.cursor, 1),
    })),
  'lineComment.extendUp': (context) =>
    updateLineComment(context, (lc) => ({
      ...lc,
      cursor: extendCursor(lc.lines, lc.cursor, -1),
    })),
  'lineComment.left': (context) =>
    updateLineComment(context, (lc) =>
      lc.layout === 'split'
        ? { ...lc, cursor: switchSide(lc.lines, lc.cursor, 'LEFT') }
        : lc,
    ),
  'lineComment.right': (context) =>
    updateLineComment(context, (lc) =>
      lc.layout === 'split'
        ? { ...lc, cursor: switchSide(lc.lines, lc.cursor, 'RIGHT') }
        : lc,
    ),
  'lineComment.compose': composeLineComment,
};
