import type React from 'react';
import { useState } from 'react';
import type { Comment } from '../types';
import { Markdown } from './Markdown';

interface CommentThreadProps {
  comments: Comment[];
  onReply?: (body: string) => void;
  onEdit?: (commentId: string, body: string) => void;
  onDelete?: (commentId: string) => void;
  currentUser?: string;
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
    <div className="border-l-2 border-primary/30 ml-4 bg-surface-container rounded-sm">
      <div
        className="flex items-center gap-2 px-3 py-1 cursor-pointer select-none text-xs text-on-surface-variant hover:text-on-surface"
        onClick={() => setIsCollapsed((prev) => !prev)}
        role="button"
        aria-expanded={!isCollapsed}
      >
        <span>{isCollapsed ? '▶' : '▼'}</span>
        <span>
          {comments.length} {comments.length === 1 ? 'comment' : 'comments'}
        </span>
      </div>

      {!isCollapsed && (
        <div>
          {comments.map((comment) => {
            const isAuthor =
              currentUser != null && currentUser === comment.author;
            const isEditing = editingId === comment.id;

            return (
              <div
                key={comment.id}
                className="px-3 py-2 border-t border-outline-variant/30 text-sm"
              >
                <div className="flex items-center justify-between mb-1">
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-xs text-on-surface">
                      @{comment.author}
                    </span>
                    <span className="text-[10px] text-on-surface-variant">
                      {new Date(comment.created_at).toLocaleDateString()}
                    </span>
                  </div>
                  {isAuthor && !isEditing && (
                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        onClick={() => {
                          setEditingId(comment.id);
                          setEditText(comment.body);
                        }}
                        className="text-[10px] text-on-surface-variant hover:text-on-surface transition-colors"
                      >
                        edit
                      </button>
                      {onDelete != null && (
                        <button
                          type="button"
                          onClick={() => onDelete(comment.id)}
                          className="text-[10px] text-error hover:underline transition-colors"
                        >
                          delete
                        </button>
                      )}
                    </div>
                  )}
                </div>

                {isEditing ? (
                  <div className="flex flex-col gap-1">
                    <textarea
                      value={editText}
                      onChange={(e) => setEditText(e.target.value)}
                      className="w-full text-xs bg-surface border border-outline-variant rounded px-2 py-1 text-on-surface resize-none"
                      rows={2}
                    />
                    <div className="flex gap-2">
                      <button
                        type="button"
                        onClick={() => handleEditSubmit(comment.id)}
                        disabled={!editText.trim()}
                        className="text-[10px] text-primary hover:underline disabled:opacity-50"
                      >
                        Save
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setEditingId(null);
                          setEditText('');
                        }}
                        className="text-[10px] text-on-surface-variant hover:text-on-surface"
                      >
                        Cancel
                      </button>
                    </div>
                  </div>
                ) : (
                  <Markdown body={comment.body} compact />
                )}
              </div>
            );
          })}

          {onReply != null && (
            <div className="px-3 py-2 border-t border-outline-variant/30">
              {showReply ? (
                <div className="flex flex-col gap-1">
                  <textarea
                    value={replyText}
                    onChange={(e) => setReplyText(e.target.value)}
                    placeholder="Write a reply..."
                    className="w-full text-xs bg-surface border border-outline-variant rounded px-2 py-1 text-on-surface resize-none"
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
                    <button
                      type="button"
                      onClick={handleReplySubmit}
                      disabled={!replyText.trim()}
                      className="text-[10px] text-primary hover:underline disabled:opacity-50"
                    >
                      Reply
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setShowReply(false);
                        setReplyText('');
                      }}
                      className="text-[10px] text-on-surface-variant hover:text-on-surface"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setShowReply(true)}
                  className="text-[10px] text-on-surface-variant hover:text-primary transition-colors"
                >
                  Reply...
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
