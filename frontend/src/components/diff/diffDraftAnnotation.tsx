import { useEffect, useRef, useState } from 'react';
import { useEffectiveBindings } from '../../hooks/useEffectiveBindings';
import { useOverlays } from '../../overlays/OverlayContext';
import { saveDraft } from '../../services/reviewDrafts';
import type { ReviewDraft } from '../../types';
import {
  isTextareaSubmit,
  TEXTAREA_SUBMIT_HOTKEY,
} from '../../utils/keybindings';
import { findActionKeys } from '../../utils/legendPresets';
import { isLineCommentCancel } from '../../utils/lineCommentActions';
import {
  cursorRange,
  type LineCommentState,
} from '../../utils/lineCommentCursor';
import { Markdown } from '../Markdown';
import { addToast } from '../ToastContainer';
import { Badge } from '../ui/badge';
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

interface DraftAnnotationCardProps {
  draft: ReviewDraft;
  annotationKey: string;
  isOutdated: boolean;
  isActive: boolean;
  onActivate: () => void;
}

/** A saved, unsubmitted line comment with effective edit/delete hints. */
export function DraftAnnotationCard({
  draft,
  annotationKey,
  isOutdated,
  isActive,
  onActivate,
}: DraftAnnotationCardProps): React.ReactElement {
  const startLine = draft.start_line === 0 ? null : draft.start_line;
  const bindings = useEffectiveBindings();
  const hints = [
    { actionId: 'detail.inlineReply', label: 'edit' },
    { actionId: 'diff.deleteDraft', label: 'delete' },
  ].flatMap(({ actionId, label }) => {
    const keys = findActionKeys(bindings, [actionId]);
    return keys === undefined ? [] : [`${keys} ${label}`];
  });
  return (
    <div
      data-annotation-key={annotationKey}
      data-active={isActive ? 'true' : 'false'}
      className={`border-l-2 bg-surface-container-low px-3 py-2 space-y-2 text-[11px] ${
        isActive
          ? 'border-primary bg-surface-container'
          : 'border-outline-variant/30'
      }`}
    >
      <button
        type="button"
        onClick={onActivate}
        aria-label={`Focus draft at ${rangeLabel(draft.path, startLine, draft.line)}`}
        className="flex w-full items-center gap-2 text-left cursor-crosshair"
      >
        <Badge variant="outline">DRAFT</Badge>
        {isOutdated && <Badge variant="outdated">OUTDATED</Badge>}
        <span className="font-mono text-[10px] text-outline truncate">
          {rangeLabel(draft.path, startLine, draft.line)}
        </span>
      </button>
      <Markdown body={draft.body} compact />
      {isActive && hints.length > 0 && (
        <p className="font-mono text-[9px] uppercase tracking-widest text-outline">
          {hints.join(' · ')}
        </p>
      )}
    </div>
  );
}

/** Inline draft composer; saving stores a pending review draft. */
export function LineCommentComposer({
  lineComment,
}: {
  lineComment: LineCommentState;
}): React.ReactElement {
  const { closeOverlay } = useOverlays();
  const bindings = useEffectiveBindings();
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
          if (isLineCommentCancel(event.nativeEvent, bindings)) return;
          event.stopPropagation();
          if (isTextareaSubmit(event)) {
            event.preventDefault();
            save();
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
