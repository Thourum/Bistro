import type React from 'react';
import { useEffect, useRef, useState } from 'react';
import { Button } from './ui/button';
import { Textarea } from './ui/textarea';

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
      <div className="mb-1.5 font-mono text-[10px] uppercase tracking-widest text-outline">
        Comment on line {lineNumber}
      </div>
      <div className="flex flex-col gap-2">
        <Textarea
          ref={textareaRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Add comment (Ctrl+Enter to send, Esc to cancel)"
          rows={3}
        />
        <div className="flex items-center justify-end gap-2">
          <Button variant="ghost" size="sm" onClick={onCancel}>
            Cancel
          </Button>
          <Button size="sm" onClick={handleSend} disabled={!text.trim()}>
            Send
          </Button>
        </div>
      </div>
    </div>
  );
}
