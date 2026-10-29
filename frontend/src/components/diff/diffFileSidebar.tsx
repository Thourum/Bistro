import { useVirtualizer } from '@tanstack/react-virtual';
import { memo, useEffect, useRef } from 'react';
import type { TriageFileStatus } from '../../types';
import type { ParsedDiffFile } from './diffPathUtils';

interface DiffFileSidebarProps {
  files: readonly ParsedDiffFile[];
  activeId: string | null;
  onSelect: (id: string) => void;
  totalCount: number;
  /** Paths marked viewed (`v`) for the current user. Empty when untracked. */
  viewedPaths?: ReadonlySet<string>;
  /** Since-last-review status per path; badges are shown for non-unchanged files. */
  triageStatuses?: ReadonlyMap<string, TriageFileStatus>;
}

// 32px wrapper: 28px h-full button + 4px pb-1 gap; rows must stay fixed-height.
const ROW_HEIGHT_PX = 32;
// Rows rendered beyond the viewport so fast scrolls don't flash blank.
const OVERSCAN_ROWS = 20;

export function DiffFileSidebar({
  files,
  activeId,
  onSelect,
  totalCount,
  viewedPaths = EMPTY_VIEWED_PATHS,
  triageStatuses,
}: DiffFileSidebarProps): React.ReactElement {
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count: files.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: OVERSCAN_ROWS,
  });

  // Keyboard file navigation (Shift+J/K) and scroll-driven activation move
  // activeId; keep its row in view.
  useEffect(() => {
    const index = files.findIndex((file) => file.id === activeId);
    if (index >= 0) virtualizer.scrollToIndex(index, { align: 'auto' });
  }, [activeId, files, virtualizer]);

  return (
    <aside className="w-1/4 min-w-[220px] bg-surface-container-low border-r border-outline-variant/20 flex flex-col min-h-0">
      <h3 className="px-4 pt-4 font-mono text-[10px] font-bold text-outline tracking-widest uppercase mb-4">
        Files Changed ({totalCount.toString().padStart(2, '0')})
      </h3>
      <div ref={scrollRef} className="flex-1 min-h-0 overflow-y-auto px-4 pb-4">
        <nav
          className="relative"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((row) => {
            const file = files[row.index];
            if (file === undefined) return null;
            return (
              <div
                key={file.id}
                className="absolute left-0 right-0 pb-1"
                style={{ top: row.start, height: ROW_HEIGHT_PX }}
              >
                <DiffFileRow
                  file={file}
                  isActive={file.id === activeId}
                  isViewed={viewedPaths.has(file.path)}
                  triageStatus={triageStatuses?.get(file.path)}
                  onSelect={onSelect}
                />
              </div>
            );
          })}
        </nav>
      </div>
    </aside>
  );
}

interface DiffFileRowProps {
  file: ParsedDiffFile;
  isActive: boolean;
  isViewed: boolean;
  triageStatus: TriageFileStatus | undefined;
  onSelect: (id: string) => void;
}

const DiffFileRow = memo(function DiffFileRow({
  file,
  isActive,
  isViewed,
  triageStatus,
  onSelect,
}: DiffFileRowProps) {
  return (
    <button
      type="button"
      onClick={() => onSelect(file.id)}
      className={`w-full h-full flex items-center gap-2 px-2 text-left cursor-crosshair transition-colors duration-150 ${
        isActive
          ? 'bg-surface-container-lowest text-primary border-l-2 border-primary'
          : 'hover:bg-surface-container-highest/50 text-on-surface border-l-2 border-transparent hover:border-primary/40'
      }`}
    >
      <span
        className={`material-symbols-outlined text-[16px] ${
          isViewed ? 'text-primary' : isActive ? 'text-primary' : 'text-outline'
        }`}
        role="img"
        aria-label={isViewed ? 'Viewed' : 'Not viewed'}
        title={isViewed ? 'Viewed' : undefined}
      >
        {isViewed ? 'check_circle' : 'description'}
      </span>
      <span
        className={`font-mono text-[11px] truncate flex-1 ${
          isViewed && !isActive ? 'text-outline' : ''
        }`}
      >
        {file.path}
      </span>
      {triageStatus !== undefined && triageStatus !== 'unchanged' && (
        <span className="font-mono text-[9px] uppercase tracking-widest text-primary shrink-0">
          {triageStatus}
        </span>
      )}
      <span className="font-mono text-[9px] text-outline shrink-0">
        +{file.additions}/-{file.deletions}
      </span>
    </button>
  );
});

const EMPTY_VIEWED_PATHS: ReadonlySet<string> = new Set();
