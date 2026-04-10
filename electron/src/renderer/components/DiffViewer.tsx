import React, { useMemo, useState } from 'react';
import type { Comment } from '../types';
import type { DiffLine, DiffSection } from '../utils/diff-parser';
import { parseDiff, parseMultiFileDiff } from '../utils/diff-parser';
import { detectLanguage, highlightCode } from '../utils/highlighter';
import { DiffFileHeader } from './DiffFileHeader';
import { ScrollArea } from './ui/scroll-area';

interface DiffViewerProps {
  diff: string;
  className?: string;
  comments?: Comment[];
  onLineClick?: (fileIndex: number, lineNumber: number) => void;
}

function getLineClasses(type: DiffLine['type']): { bg: string; text: string } {
  switch (type) {
    case 'added':
      return {
        bg: 'bg-green-50 dark:bg-green-900/20',
        text: 'text-green-900 dark:text-green-100',
      };
    case 'removed':
      return {
        bg: 'bg-red-50 dark:bg-red-900/20',
        text: 'text-red-900 dark:text-red-100',
      };
    default:
      return { bg: 'bg-surface-container-lowest', text: 'text-on-surface' };
  }
}

function getLinePrefix(type: DiffLine['type']): string {
  if (type === 'added') return '+';
  if (type === 'removed') return '-';
  return ' ';
}

function extractFileName(section: DiffSection): string {
  const path = section.newFile || section.oldFile;
  return path.replace(/^[ab]\//, '');
}

function getLineNumber(line: DiffLine): number | undefined {
  return line.newLineNumber ?? line.oldLineNumber;
}

interface FileSectionProps {
  section: DiffSection;
  fileIndex: number;
  isSelected: boolean;
  isCollapsed: boolean;
  comments: Comment[];
  onToggleCollapse: () => void;
  onFileClick: () => void;
  onLineClick?: (fileIndex: number, lineNumber: number) => void;
}

function FileSection({
  section,
  fileIndex,
  isSelected,
  isCollapsed,
  comments,
  onToggleCollapse,
  onFileClick,
  onLineClick,
}: FileSectionProps): React.ReactElement {
  const fileName = extractFileName(section);
  const language = detectLanguage(fileName);

  // Group comments by line number for fast lookup
  const commentsByLine = useMemo(() => {
    const map = new Map<number, Comment[]>();
    for (const comment of comments) {
      if (comment.file === fileName && comment.line != null) {
        const existing = map.get(comment.line) ?? [];
        map.set(comment.line, [...existing, comment]);
      }
    }
    return map;
  }, [comments, fileName]);

  return (
    <div className="border-b border-outline-variant">
      <DiffFileHeader
        section={section}
        index={fileIndex}
        isSelected={isSelected}
        isCollapsed={isCollapsed}
        onToggleCollapse={onToggleCollapse}
        onClick={onFileClick}
      />

      {!isCollapsed && (
        <div className="font-mono text-sm bg-surface-container-lowest">
          {section.hunks.map((hunk, hunkIdx) => (
            <div key={`hunk-${hunk.header}-${hunkIdx}`}>
              <div className="bg-surface-container px-4 py-1 text-on-surface-variant font-bold border-b border-outline-variant">
                {hunk.header}
              </div>

              {hunk.lines.map((line, lineIdx) => {
                const { bg, text } = getLineClasses(line.type);
                const prefix = getLinePrefix(line.type);
                const lineNum = getLineNumber(line);
                const lineComments =
                  lineNum != null ? (commentsByLine.get(lineNum) ?? []) : [];
                // Only syntax-highlight added/removed lines; context lines stay as plain text
                // so text-content based queries (getByText) continue to work in tests and for
                // screen readers.
                const useHighlight =
                  line.type === 'added' || line.type === 'removed';
                const highlighted = useHighlight
                  ? highlightCode(line.content, language)
                  : null;

                return (
                  <React.Fragment key={`line-${line.type}-${lineIdx}`}>
                    <div
                      className={`diff-${line.type} flex border-b border-outline-variant/30 ${bg} ${onLineClick != null ? 'cursor-pointer hover:brightness-95' : ''}`}
                      onClick={() => {
                        if (onLineClick != null && lineNum != null) {
                          onLineClick(fileIndex, lineNum);
                        }
                      }}
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
                      {highlighted != null ? (
                        <div
                          className={`flex-1 px-3 py-1 overflow-auto ${text} whitespace-pre-wrap break-words`}
                          // highlight.js escapes HTML internally — safe to use dangerouslySetInnerHTML
                          dangerouslySetInnerHTML={{ __html: highlighted }}
                        />
                      ) : (
                        <div
                          className={`flex-1 px-3 py-1 overflow-auto ${text} whitespace-pre-wrap break-words`}
                        >
                          {line.content}
                        </div>
                      )}
                    </div>

                    {lineComments.map((comment) => (
                      <div
                        key={comment.id}
                        className="border-b border-outline-variant bg-surface-container px-4 py-2 text-sm"
                      >
                        <span className="font-bold text-primary text-xs">
                          @{comment.author}
                        </span>
                        <span className="mx-2 text-on-surface-variant text-xs">
                          {new Date(comment.created_at).toLocaleDateString()}
                        </span>
                        <p className="text-on-surface mt-1">{comment.body}</p>
                      </div>
                    ))}
                  </React.Fragment>
                );
              })}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function DiffViewer({
  diff,
  className = '',
  comments = [],
  onLineClick,
}: DiffViewerProps) {
  // Support both multi-file git diffs and single-file unified diffs
  const sections = useMemo((): DiffSection[] => {
    if (!diff || diff.trim() === '') return [];
    const multi = parseMultiFileDiff(diff);
    if (multi.length > 0) return multi;
    // Fall back to single-file parsing
    const single = parseDiff(diff);
    if (single.hunks.length > 0 || single.oldFile || single.newFile)
      return [single];
    return [];
  }, [diff]);

  const [selectedFileIndex, setSelectedFileIndex] = useState(0);
  const [collapsedFiles, setCollapsedFiles] = useState<Set<number>>(new Set());

  if (!diff || diff.trim() === '') {
    return (
      <div className="p-4 text-center text-on-surface-variant">No changes</div>
    );
  }

  if (sections.length === 0) {
    return (
      <div className="p-4 text-center text-on-surface-variant">No changes</div>
    );
  }

  const handleToggleCollapse = (index: number) => {
    setCollapsedFiles((prev) => {
      const next = new Set(prev);
      if (next.has(index)) {
        next.delete(index);
      } else {
        next.add(index);
      }
      return next;
    });
  };

  return (
    <ScrollArea
      data-diff-viewport
      className={`w-full h-full border border-outline-variant overflow-auto ${className}`}
    >
      <div className="w-full">
        {sections.map((section, idx) => (
          <FileSection
            key={`file-${section.newFile || section.oldFile}-${idx}`}
            section={section}
            fileIndex={idx}
            isSelected={idx === selectedFileIndex}
            isCollapsed={collapsedFiles.has(idx)}
            comments={comments}
            onToggleCollapse={() => handleToggleCollapse(idx)}
            onFileClick={() => setSelectedFileIndex(idx)}
            {...(onLineClick !== undefined && { onLineClick })}
          />
        ))}
      </div>
    </ScrollArea>
  );
}
