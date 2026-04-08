import React, { useMemo } from 'react';
import { ScrollArea } from './ui/scroll-area';
import { parseDiff } from '../utils/diff-parser';
import type { DiffLine } from '../utils/diff-parser';

interface DiffViewerProps {
  diff: string;
  className?: string;
}

function getLineClasses(type: DiffLine['type']): { bg: string; text: string } {
  switch (type) {
    case 'added':
      return { bg: 'bg-green-50 dark:bg-green-900/20', text: 'text-green-900 dark:text-green-100' };
    case 'removed':
      return { bg: 'bg-red-50 dark:bg-red-900/20', text: 'text-red-900 dark:text-red-100' };
    default:
      return { bg: 'bg-surface-container-lowest', text: 'text-on-surface' };
  }
}

function getLinePrefix(type: DiffLine['type']): string {
  if (type === 'added') return '+';
  if (type === 'removed') return '-';
  return ' ';
}

export function DiffViewer({ diff, className = '' }: DiffViewerProps) {
  if (!diff || diff.trim() === '') {
    return (
      <div className="p-4 text-center text-on-surface-variant">
        No changes
      </div>
    );
  }

  const parsed = useMemo(() => parseDiff(diff), [diff]);

  return (
    <ScrollArea className={`w-full h-full border border-outline-variant overflow-auto ${className}`}>
      <div className="font-mono text-sm bg-surface-container-lowest">
        {parsed.oldFile && (
          <div className="bg-surface-container-low border-b border-outline-variant px-4 py-2 text-on-surface-variant">
            --- {parsed.oldFile}
          </div>
        )}
        {parsed.newFile && (
          <div className="bg-surface-container-low border-b border-outline-variant px-4 py-2 text-on-surface-variant">
            +++ {parsed.newFile}
          </div>
        )}

        {parsed.hunks.map((hunk, hunkIdx) => (
          <div key={`hunk-${hunk.header}-${hunkIdx}`}>
            <div className="bg-surface-container px-4 py-1 text-on-surface-variant font-bold border-b border-outline-variant">
              {hunk.header}
            </div>

            {hunk.lines.map((line, lineIdx) => {
              const { bg, text } = getLineClasses(line.type);
              const prefix = getLinePrefix(line.type);

              return (
                <div
                  key={`line-${line.type}-${lineIdx}`}
                  className={`diff-${line.type} flex border-b border-outline-variant/30 ${bg}`}
                >
                  <div className="w-12 flex-shrink-0 px-2 py-1 text-right text-on-surface-variant border-r border-outline-variant/50 select-none bg-surface-container-low">
                    {line.oldLineNumber ?? '\u00a0'}
                  </div>
                  <div className="w-12 flex-shrink-0 px-2 py-1 text-right text-on-surface-variant border-r border-outline-variant/50 select-none bg-surface-container-low">
                    {line.newLineNumber ?? '\u00a0'}
                  </div>
                  <div className="w-4 flex-shrink-0 px-1 py-1 select-none text-on-surface-variant text-center">
                    {prefix}
                  </div>
                  <div className={`flex-1 px-3 py-1 overflow-auto ${text} whitespace-pre-wrap break-words`}>
                    {line.content}
                  </div>
                </div>
              );
            })}
          </div>
        ))}
      </div>
    </ScrollArea>
  );
}
