import type React from 'react';
import { useState } from 'react';
import type { Comment, CommentSeverity } from '../types';
import { humanizeSource } from '../utils/classification';
import { Markdown } from './Markdown';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Textarea } from './ui/textarea';

interface CommentThreadProps {
  comments: Comment[];
  onReply?: (body: string) => void;
  onEdit?: (commentId: string, body: string) => void;
  onDelete?: (commentId: string) => void;
  currentUser?: string;
}

const COMMENT_SEVERITY_STRIPE_CLASSES: Record<
  CommentSeverity | 'none',
  string
> = {
  critical: 'border-l-error',
  warning: 'border-l-tertiary',
  suggestion: 'border-l-primary',
  nit: 'border-l-secondary',
  info: 'border-l-outline',
  none: 'border-l-outline-variant/30',
};

function formatClassificationLabel(value: string | undefined): string {
  if (value === undefined || value.trim().length === 0) return 'unclassified';
  return value;
}

function classificationMetadata(comment: Comment): React.ReactElement | null {
  // Unclassified comments render no badges — classification is surfaced
  // only where it exists, not as noise on every human comment.
  if (!comment.severity && !comment.bot_vendor) return null;

  const severityLabel = formatClassificationLabel(comment.severity);
  const vendorLabel = formatClassificationLabel(comment.bot_vendor);
  const whyTooltip = humanizeSource(comment.classification_source);

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <Badge
        variant="outline"
        title={whyTooltip}
        aria-label={`${severityLabel}: ${whyTooltip}`}
      >
        {severityLabel}
      </Badge>
      <Badge variant="neutral">{vendorLabel}</Badge>
    </div>
  );
}

export function CommentThread({
  comments,
  onReply,
  onEdit,
  onDelete,
  currentUser,
}: CommentThreadProps): React.ReactElement | null {
  const [isCollapsed, setIsCollapsed] = useState(false);
  const [replyText, setReplyText] = useState('');
  const [showReply, setShowReply] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editText, setEditText] = useState('');

  if (comments.length === 0) {
    return null;
  }

  const handleReplySubmit = () => {
    const trimmed = replyText.trim();
    if (!trimmed || onReply == null) return;
    onReply(trimmed);
    setReplyText('');
    setShowReply(false);
  };

  const handleEditSubmit = (commentId: string) => {
    const trimmed = editText.trim();
    if (!trimmed || onEdit == null) return;
    onEdit(commentId, trimmed);
    setEditingId(null);
    setEditText('');
  };

  return (
    <div className="ml-4 border-l-2 border-primary/40 bg-surface-container-low">
      <button
        type="button"
        className="flex w-full cursor-crosshair select-none items-center gap-2 border-0 px-3 py-1.5 text-left font-mono text-[10px] uppercase tracking-widest text-outline transition-colors duration-150 hover:text-on-surface"
        onClick={() => setIsCollapsed((prev) => !prev)}
        aria-expanded={!isCollapsed}
      >
        <span>{isCollapsed ? '▶' : '▼'}</span>
        <span>
          {comments.length} {comments.length === 1 ? 'comment' : 'comments'}
        </span>
      </button>

      {!isCollapsed && (
        <div>
          {comments.map((comment) => {
            const isAuthor =
              currentUser != null && currentUser === comment.author;
            const isEditing = editingId === comment.id;
            const severity = comment.severity ?? 'none';

            return (
              <div
                key={comment.id}
                data-severity={severity}
                className={`border-t border-l-2 border-t-outline-variant/30 px-3 py-2.5 text-sm ${COMMENT_SEVERITY_STRIPE_CLASSES[severity]}`}
              >
                <div className="mb-1.5 flex items-center justify-between gap-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-[10px] font-bold uppercase tracking-widest text-on-surface">
                      @{comment.author}
                    </span>
                    <span className="font-mono text-[10px] text-outline">
                      {new Date(comment.created_at).toLocaleDateString()}
                    </span>
                    {classificationMetadata(comment)}
                  </div>
                  {isAuthor && !isEditing && (
                    <div className="flex items-center gap-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-auto px-1.5 py-0.5 normal-case tracking-widest hover:text-on-surface"
                        onClick={() => {
                          setEditingId(comment.id);
                          setEditText(comment.body);
                        }}
                      >
                        edit
                      </Button>
                      {onDelete != null && (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-auto px-1.5 py-0.5 normal-case tracking-widest text-error hover:text-error hover:underline"
                          onClick={() => onDelete(comment.id)}
                        >
                          delete
                        </Button>
                      )}
                    </div>
                  )}
                </div>

                {isEditing ? (
                  <div className="flex flex-col gap-2">
                    <Textarea
                      value={editText}
                      onChange={(e) => setEditText(e.target.value)}
                      className="text-xs"
                      rows={2}
                    />
                    <div className="flex gap-2">
                      <Button
                        variant="link"
                        size="sm"
                        className="h-auto px-0"
                        onClick={() => handleEditSubmit(comment.id)}
                        disabled={!editText.trim()}
                      >
                        Save
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-auto px-0 hover:text-on-surface"
                        onClick={() => {
                          setEditingId(null);
                          setEditText('');
                        }}
                      >
                        Cancel
                      </Button>
                    </div>
                  </div>
                ) : (
                  <Markdown body={comment.body} compact />
                )}
              </div>
            );
          })}

          {onReply != null && (
            <div className="border-t border-outline-variant/30 px-3 py-2">
              {showReply ? (
                <div className="flex flex-col gap-2">
                  <Textarea
                    value={replyText}
                    onChange={(e) => setReplyText(e.target.value)}
                    placeholder="Write a reply…"
                    className="text-xs"
                    rows={2}
                    onKeyDown={(e) => {
                      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
                        e.preventDefault();
                        handleReplySubmit();
                      }
                      if (e.key === 'Escape') {
                        setShowReply(false);
                        setReplyText('');
                      }
                    }}
                  />
                  <div className="flex gap-2">
                    <Button
                      variant="link"
                      size="sm"
                      className="h-auto px-0"
                      onClick={handleReplySubmit}
                      disabled={!replyText.trim()}
                    >
                      Reply
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-auto px-0 hover:text-on-surface"
                      onClick={() => {
                        setShowReply(false);
                        setReplyText('');
                      }}
                    >
                      Cancel
                    </Button>
                  </div>
                </div>
              ) : (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-auto px-0 hover:text-primary"
                  onClick={() => setShowReply(true)}
                >
                  Reply...
                </Button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
