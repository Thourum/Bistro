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

export function DiffFileSidebar({
  files,
  activeId,
  onSelect,
  totalCount,
  viewedPaths = EMPTY_VIEWED_PATHS,
  triageStatuses,
}: DiffFileSidebarProps): React.ReactElement {
  return (
    <aside className="w-1/4 min-w-[220px] bg-surface-container-low border-r border-outline-variant/20 overflow-y-auto">
      <div className="p-4">
        <h3 className="font-mono text-[10px] font-bold text-outline tracking-widest uppercase mb-4">
          Files Changed ({totalCount.toString().padStart(2, '0')})
        </h3>
        <nav className="space-y-1">
          {files.map((file) => {
            const isActive = file.id === activeId;
            const isViewed = viewedPaths.has(file.path);
            const triageStatus = triageStatuses?.get(file.path);
            return (
              <button
                key={file.id}
                type="button"
                onClick={() => onSelect(file.id)}
                className={`w-full flex items-center gap-2 px-2 py-1.5 text-left cursor-crosshair transition-colors duration-150 ${
                  isActive
                    ? 'bg-surface-container-lowest text-primary border-l-2 border-primary'
                    : 'hover:bg-surface-container-highest/50 text-on-surface border-l-2 border-transparent hover:border-primary/40'
                }`}
              >
                <span
                  className={`material-symbols-outlined text-[16px] ${
                    isViewed
                      ? 'text-primary'
                      : isActive
                        ? 'text-primary'
                        : 'text-outline'
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
          })}
        </nav>
      </div>
    </aside>
  );
}

const EMPTY_VIEWED_PATHS: ReadonlySet<string> = new Set();
