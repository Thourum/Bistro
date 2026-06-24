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
    <div className="ml-4 border-l-2 border-primary/40 bg-surface-container-low">
      <div
        className="flex cursor-crosshair select-none items-center gap-2 px-3 py-1.5 font-mono text-[10px] uppercase tracking-widest text-outline transition-colors duration-150 hover:text-on-surface"
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
                className="border-t border-outline-variant/30 px-3 py-2.5 text-sm"
              >
                <div className="mb-1.5 flex items-center justify-between">
                  <div className="flex items-baseline gap-2">
                    <span className="font-mono text-[10px] font-bold uppercase tracking-widest text-on-surface">
                      @{comment.author}
                    </span>
                    <span className="font-mono text-[10px] text-outline">
                      {new Date(comment.created_at).toLocaleDateString()}
                    </span>
                  </div>
                  {isAuthor && !isEditing && (
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={() => {
                          setEditingId(comment.id);
                          setEditText(comment.body);
                        }}
                        className="font-mono text-[10px] uppercase tracking-widest text-outline transition-colors duration-150 hover:text-on-surface"
                      >
                        edit
                      </button>
                      {onDelete != null && (
                        <button
                          type="button"
                          onClick={() => onDelete(comment.id)}
                          className="font-mono text-[10px] uppercase tracking-widest text-error transition-colors duration-150 hover:underline"
                        >
                          delete
                        </button>
                      )}
                    </div>
                  )}
                </div>

                {isEditing ? (
                  <div className="flex flex-col gap-2">
                    <textarea
                      value={editText}
                      onChange={(e) => setEditText(e.target.value)}
                      className="w-full resize-none border border-outline-variant bg-surface px-2 py-1 font-mono text-xs text-on-surface focus:border-primary/60 focus:outline-none"
                      rows={2}
                    />
                    <div className="flex gap-3">
                      <button
                        type="button"
                        onClick={() => handleEditSubmit(comment.id)}
                        disabled={!editText.trim()}
                        className="font-mono text-[10px] uppercase tracking-widest text-primary transition-colors duration-150 hover:underline disabled:opacity-50"
                      >
                        Save
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          setEditingId(null);
                          setEditText('');
                        }}
                        className="font-mono text-[10px] uppercase tracking-widest text-outline transition-colors duration-150 hover:text-on-surface"
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
            <div className="border-t border-outline-variant/30 px-3 py-2">
              {showReply ? (
                <div className="flex flex-col gap-2">
                  <textarea
                    value={replyText}
                    onChange={(e) => setReplyText(e.target.value)}
                    placeholder="Write a reply…"
                    className="w-full resize-none border border-outline-variant bg-surface px-2 py-1 font-mono text-xs text-on-surface focus:border-primary/60 focus:outline-none"
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
                  <div className="flex gap-3">
                    <button
                      type="button"
                      onClick={handleReplySubmit}
                      disabled={!replyText.trim()}
                      className="font-mono text-[10px] uppercase tracking-widest text-primary transition-colors duration-150 hover:underline disabled:opacity-50"
                    >
                      Reply
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setShowReply(false);
                        setReplyText('');
                      }}
                      className="font-mono text-[10px] uppercase tracking-widest text-outline transition-colors duration-150 hover:text-on-surface"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setShowReply(true)}
                  className="font-mono text-[10px] uppercase tracking-widest text-outline transition-colors duration-150 hover:text-primary"
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
