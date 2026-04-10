import type React from 'react';
import type { DiffSection } from '../utils/diff-parser';

interface DiffFileHeaderProps {
  section: DiffSection;
  index: number;
  isSelected: boolean;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  onClick: () => void;
}

function countChanges(section: DiffSection): {
  additions: number;
  deletions: number;
} {
  let additions = 0;
  let deletions = 0;
  for (const hunk of section.hunks) {
    for (const line of hunk.lines) {
      if (line.type === 'added') additions++;
      if (line.type === 'removed') deletions++;
    }
  }
  return { additions, deletions };
}

function extractFileName(filePath: string): string {
  return filePath.replace(/^[ab]\//, '');
}

export function DiffFileHeader({
  section,
  index,
  isSelected,
  isCollapsed,
  onToggleCollapse,
  onClick,
}: DiffFileHeaderProps): React.ReactElement {
  const fileName = extractFileName(section.newFile || section.oldFile);
  const { additions, deletions } = countChanges(section);

  return (
    <div
      onClick={onClick}
      data-testid={`diff-file-header-${index}`}
      className={`flex items-center justify-between px-4 py-2 border-b border-outline-variant cursor-pointer transition-colors select-none ${
        isSelected
          ? 'bg-primary/10 text-primary'
          : 'bg-surface-container-low text-on-surface hover:bg-surface-container'
      }`}
    >
      <div className="flex items-center gap-2 min-w-0">
        <button
          onClick={(e) => {
            e.stopPropagation();
            onToggleCollapse();
          }}
          aria-label={isCollapsed ? 'Expand file' : 'Collapse file'}
          className="flex-shrink-0 text-on-surface-variant hover:text-on-surface transition-colors"
        >
          {isCollapsed ? '▶' : '▼'}
        </button>
        <span className="font-mono text-xs truncate" title={fileName}>
          {fileName}
        </span>
      </div>
      <div className="flex items-center gap-2 flex-shrink-0 font-mono text-xs">
        {additions > 0 && (
          <span className="text-green-600 dark:text-green-400">
            +{additions}
          </span>
        )}
        {deletions > 0 && (
          <span className="text-red-600 dark:text-red-400">-{deletions}</span>
        )}
      </div>
    </div>
  );
}
