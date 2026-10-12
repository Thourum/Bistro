import { useEffect, useRef, useState } from 'react';
import type {
  Comment,
  CommentSeverity,
  ReviewThread,
  ThreadLifecycle,
} from '../../types';
import { threadLifecycle } from '../../types';
import { humanizeSource } from '../../utils/classification';
import { Markdown } from '../Markdown';
import { type StatusKind, StatusSquare } from '../StatusSquare';
import { Badge } from '../ui/badge';
import { Button } from '../ui/button';
import { ShortcutBadge } from '../ui/shortcut-badge';
import { Textarea } from '../ui/textarea';

const LIFECYCLE_SQUARE = {
  open: 'running',
  resolved: 'success',
  outdated: 'neutral',
  unknown: 'neutral',
} satisfies Record<ThreadLifecycle, StatusKind>;

const LIFECYCLE_LABEL = {
  open: 'Open',
  resolved: 'Resolved',
  outdated: 'Outdated',
  unknown: 'Unknown',
} satisfies Record<ThreadLifecycle, string>;

const COMMENT_SEVERITY_STRIPE_CLASSES = {
  critical: 'border-l-error',
  warning: 'border-l-tertiary',
  suggestion: 'border-l-primary',
  nit: 'border-l-secondary',
  info: 'border-l-outline',
  none: 'border-l-outline-variant/30',
} satisfies Record<CommentSeverity | 'none', string>;

function formatClassificationLabel(value: string | undefined): string {
  if (value === undefined || value.trim().length === 0) return 'unclassified';
  return value;
}

interface ThreadAnnotationCardProps {
  thread: ReviewThread | undefined;
  comments: readonly Comment[];
  isActive?: boolean;
  isReplying?: boolean;
  onActivate?: () => void;
  onReplyClose?: () => void;
  onThreadReply?: (threadId: string, body: string) => void;
}

export function ThreadAnnotationCard({
  thread,
  comments,
  isActive = false,
  isReplying = false,
  onActivate,
  onReplyClose,
  onThreadReply,
}: ThreadAnnotationCardProps): React.ReactElement | null {
  if (thread === undefined) return null;
  const lifecycle = threadLifecycle(thread);
  const isDimmed =
    !isActive && (lifecycle === 'resolved' || lifecycle === 'outdated');

  return (
    <div
      data-annotation-key={thread.id}
      data-testid="thread-annotation-card"
      data-thread-id={thread.id}
      data-active={isActive ? 'true' : 'false'}
      data-replying={isReplying ? 'true' : 'false'}
      className={`border-l-2 bg-surface-container-low px-3 py-2 space-y-2 text-[11px] transition-colors duration-150 ${
        isActive
          ? 'border-primary bg-surface-container'
          : 'border-outline-variant/30'
      } ${isDimmed ? 'opacity-50' : ''}`}
    >
      <button
        type="button"
        onClick={onActivate}
        disabled={onActivate === undefined}
        aria-label={`Focus review thread at ${thread.file_path}:${thread.line}`}
        className="flex w-full items-center gap-2 text-left cursor-crosshair disabled:cursor-default"
      >
        <Badge variant={lifecycle}>
          <StatusSquare
            status={LIFECYCLE_SQUARE[lifecycle]}
            size="w-2 h-2"
            label={LIFECYCLE_LABEL[lifecycle]}
          />
          {LIFECYCLE_LABEL[lifecycle]}
        </Badge>
        <span className="font-mono text-[10px] text-outline truncate">
          {thread.file_path}
          <span className="text-outline-variant">:{thread.line}</span>
        </span>
      </button>

      {comments.length > 0 ? (
        <ol className="space-y-2 pl-2 border-l border-outline-variant/20">
          {comments.map((comment) => {
            const severity = comment.severity ?? 'none';
            const sourceTooltip = humanizeSource(comment.classification_source);

            return (
              <li
                key={comment.id}
                data-severity={severity}
                className={`space-y-1 list-none border-l-2 pl-2 ${COMMENT_SEVERITY_STRIPE_CLASSES[severity]}`}
              >
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="font-mono text-[10px] font-bold uppercase tracking-widest text-on-surface">
                    @{comment.author}
                  </span>
                  {(comment.severity || comment.bot_vendor) && (
                    <>
                      <Badge
                        variant="outline"
                        title={sourceTooltip}
                        aria-label={`${formatClassificationLabel(comment.severity)}: ${sourceTooltip}`}
                      >
                        {formatClassificationLabel(comment.severity)}
                      </Badge>
                      <Badge variant="neutral">
                        {formatClassificationLabel(comment.bot_vendor)}
                      </Badge>
                    </>
                  )}
                </div>
                <Markdown body={comment.body} compact />
              </li>
            );
          })}
        </ol>
      ) : (
        <p className="font-mono text-[10px] text-outline-variant pl-2">
          No comment text available for this thread.
        </p>
      )}

      {isReplying && (
        <ReplyComposer
          threadId={thread.id}
          {...(onThreadReply !== undefined ? { onSubmit: onThreadReply } : {})}
          {...(onReplyClose !== undefined ? { onClose: onReplyClose } : {})}
        />
      )}
    </div>
  );
}

interface ReplyComposerProps {
  threadId: string;
  onSubmit?: (threadId: string, body: string) => void;
  onClose?: () => void;
}

function ReplyComposer({
  threadId,
  onSubmit,
  onClose,
}: ReplyComposerProps): React.ReactElement {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [body, setBody] = useState('');
  const trimmed = body.trim();

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const closeAndBlur = (): void => {
    textareaRef.current?.blur();
    onClose?.();
  };

  const submit = (): void => {
    if (trimmed.length === 0 || onSubmit === undefined) return;
    onSubmit(threadId, trimmed);
    setBody('');
    closeAndBlur();
  };

  return (
    <div
      className="mt-2 space-y-2 border border-outline-variant/40 bg-surface p-3"
      data-testid="reply-composer"
      data-thread-id={threadId}
    >
      <label
        className="font-mono text-[9px] font-bold uppercase tracking-widest text-outline"
        htmlFor={`reply-textarea-${threadId}`}
      >
        Reply to thread
      </label>
      <Textarea
        ref={textareaRef}
        id={`reply-textarea-${threadId}`}
        value={body}
        onChange={(event) => setBody(event.target.value)}
        rows={3}
        placeholder="Write a reply…"
        className="bg-surface-container text-xs"
        aria-label="Reply to thread"
        onKeyDown={(event) => {
          event.stopPropagation();
          if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
            event.preventDefault();
            submit();
          }
          if (event.key === 'Escape') {
            event.preventDefault();
            closeAndBlur();
          }
        }}
      />
      <div className="flex justify-end gap-2">
        <Button variant="outline" size="sm" onClick={closeAndBlur}>
          Cancel
        </Button>
        <Button
          variant="default"
          size="sm"
          onClick={submit}
          disabled={trimmed.length === 0 || onSubmit === undefined}
        >
          Reply
          <ShortcutBadge hotkey="Mod+Enter" />
        </Button>
      </div>
    </div>
  );
}
