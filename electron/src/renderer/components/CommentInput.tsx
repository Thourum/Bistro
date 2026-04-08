import React, { useState, useRef, useEffect } from 'react';

interface CommentInputProps {
  fileIndex: number;
  lineNumber: number;
  onSubmit: (body: string) => void;
  onCancel: () => void;
}

export function CommentInput({
  fileIndex,
  lineNumber,
  onSubmit,
  onCancel,
}: CommentInputProps): React.ReactElement {
  const [text, setText] = useState('');
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      const trimmed = text.trim();
      if (trimmed) onSubmit(trimmed);
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      onCancel();
    }
  };

  const handleSend = () => {
    const trimmed = text.trim();
    if (trimmed) onSubmit(trimmed);
  };

  return (
    <div
      className="border-t border-outline-variant bg-surface-container p-3"
      data-testid={`comment-input-file-${fileIndex}-line-${lineNumber}`}
    >
      <div className="text-[10px] text-on-surface-variant mb-1 font-mono">
        Comment on line {lineNumber}
      </div>
      <div className="flex flex-col gap-2">
        <textarea
          ref={textareaRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Add comment (Ctrl+Enter to send, Esc to cancel)"
          rows={3}
          className="w-full text-sm bg-surface border border-outline-variant rounded px-3 py-2 text-on-surface resize-none focus:outline-none focus:ring-1 focus:ring-primary"
        />
        <div className="flex items-center gap-3 justify-end">
          <button
            onClick={onCancel}
            className="text-xs text-on-surface-variant hover:text-on-surface transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleSend}
            disabled={!text.trim()}
            className="text-xs text-primary hover:underline disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            Send
          </button>
        </div>
      </div>
    </div>
  );
}
