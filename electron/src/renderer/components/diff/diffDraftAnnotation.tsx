import { useEffect, useRef, useState } from 'react';
import { useOverlays } from '../../overlays/OverlayContext';
import { saveDraft } from '../../services/reviewDrafts';
import {
  isTextareaSubmit,
  TEXTAREA_SUBMIT_HOTKEY,
} from '../../utils/keybindings';
import {
  cursorRange,
  type LineCommentState,
} from '../../utils/lineCommentCursor';
import { addToast } from '../ToastContainer';
import { Button } from '../ui/button';
import { ShortcutBadge } from '../ui/shortcut-badge';
import { Textarea } from '../ui/textarea';
import { COMPOSER_ANNOTATION_KEY } from './diffAnnotations';

function rangeLabel(
  path: string,
  startLine: number | null,
  line: number,
): string {
  return startLine === null
    ? `${path}:${line}`
    : `${path}:${startLine}-${line}`;
}

/** Inline draft composer; saving stores a pending review draft. */
export function LineCommentComposer({
  lineComment,
}: {
  lineComment: LineCommentState;
}): React.ReactElement {
  const { closeOverlay } = useOverlays();
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [body, setBody] = useState(lineComment.composer?.initialBody ?? '');
  const [isSaving, setIsSaving] = useState(false);
  const range = cursorRange(lineComment.cursor);
  const trimmed = body.trim();

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const save = (): void => {
    if (trimmed.length === 0 || isSaving) return;
    setIsSaving(true);
    void saveDraft({
      prId: lineComment.prId,
      id: lineComment.composer?.draftId ?? null,
      path: lineComment.path,
      side: range.side,
      startLine: range.startLine,
      line: range.line,
      body: trimmed,
      commitSha: lineComment.headSha,
    }).match(
      () => closeOverlay(),
      (fault) => {
        setIsSaving(false);
        addToast({
          type: 'error',
          message: `Draft not saved: ${fault.message}`,
          dismissible: true,
        });
      },
    );
  };

  return (
    <div
      data-annotation-key={COMPOSER_ANNOTATION_KEY}
      className="mt-2 space-y-2 border border-primary/60 bg-surface p-3"
    >
      <label
        htmlFor="line-comment-textarea"
        className="font-mono text-[9px] font-bold uppercase tracking-widest text-outline"
      >
        Draft comment ·{' '}
        {rangeLabel(lineComment.path, range.startLine, range.line)} (
        {range.side === 'LEFT' ? 'old' : 'new'})
      </label>
      <Textarea
        ref={textareaRef}
        id="line-comment-textarea"
        value={body}
        onChange={(event) => setBody(event.target.value)}
        rows={3}
        placeholder="Write a comment…"
        className="bg-surface-container text-xs"
        aria-label="Draft line comment"
        onKeyDown={(event) => {
          event.stopPropagation();
          if (isTextareaSubmit(event)) {
            event.preventDefault();
            save();
          }
          if (event.key === 'Escape') {
            event.preventDefault();
            closeOverlay();
          }
        }}
      />
      <div className="flex justify-end gap-2">
        <Button variant="outline" size="sm" onClick={closeOverlay}>
          Discard
        </Button>
        <Button
          variant="default"
          size="sm"
          onClick={save}
          disabled={trimmed.length === 0 || isSaving}
        >
          Save draft
          <ShortcutBadge hotkey={TEXTAREA_SUBMIT_HOTKEY} />
        </Button>
      </div>
    </div>
  );
}
