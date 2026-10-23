import type { ReactNode } from 'react';
import { RangeDiffView } from '../components/RangeDiffView';
import { StatusSquare } from '../components/StatusSquare';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import type { DiffFile, PR, RangeDiffResult } from '../types';
import { findActionKeys } from '../utils/legendPresets';

interface DiffPageToolbarProps {
  pr: PR | undefined;
  showRangeDiff: boolean;
  sinceLastReview: boolean;
  selectedPatchsetPair: readonly [number, number] | null;
  findQuery: string;
  onFindQueryChange: (query: string) => void;
  onBack: () => void;
}

export function DiffPageToolbar({
  pr,
  showRangeDiff,
  sinceLastReview,
  selectedPatchsetPair,
  findQuery,
  onFindQueryChange,
  onBack,
}: DiffPageToolbarProps): React.ReactElement {
  return (
    <div className="shrink-0 h-10 flex items-center justify-between px-4 border-b border-outline-variant/30 bg-surface-container-lowest">
      <div className="flex items-center gap-4 min-w-0">
        <button
          type="button"
          onClick={onBack}
          className="font-mono text-[10px] uppercase tracking-widest text-outline hover:text-on-surface transition-colors"
        >
          ← Back
        </button>
        {pr !== undefined && (
          <>
            <span className="font-mono text-[11px] font-bold tracking-widest text-primary">
              PR-{pr.number}
            </span>
            <span className="font-mono text-[10px] text-outline truncate">
              {pr.head_branch}
              <span className="text-outline-variant"> → </span>
              {pr.base_branch}
            </span>
            {showRangeDiff && (
              <span className="font-mono text-[10px] uppercase tracking-widest text-primary border border-primary/40 px-2 py-0.5">
                {sinceLastReview
                  ? 'Since last review'
                  : `P${selectedPatchsetPair?.[0]} → P${selectedPatchsetPair?.[1]}`}
              </span>
            )}
          </>
        )}
      </div>
      <div className="flex items-center gap-3">
        {!showRangeDiff && (
          <input
            type="text"
            value={findQuery}
            onChange={(event) => onFindQueryChange(event.target.value)}
            placeholder="Find in diff…"
            className="font-mono text-[11px] px-3 py-1 w-64 bg-surface border border-outline-variant/40 placeholder:text-outline-variant"
          />
        )}
        <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
          ESC to exit
        </span>
      </div>
    </div>
  );
}

interface DiffPageStatusBarProps {
  diff: string;
  files: DiffFile[];
  currentFile: DiffFile | undefined;
  showRangeDiff: boolean;
  rangeResult: RangeDiffResult | null;
}

export function DiffPageStatusBar({
  diff,
  files,
  currentFile,
  showRangeDiff,
  rangeResult,
}: DiffPageStatusBarProps): React.ReactElement {
  const viewedCount = files.filter((file) => file.viewed).length;
  const markViewedKeys = findActionKeys(useEffectiveBindings(), [
    'diff.markViewed',
  ]);
  return (
    <div className="shrink-0 h-6 px-4 flex items-center justify-between border-t border-outline-variant/30 bg-surface-container-low">
      <div className="flex items-center gap-4 font-mono text-[10px] uppercase tracking-widest text-outline">
        <span>
          {showRangeDiff && rangeResult !== null
            ? `${rangeResult.raw.split('\n').length} lines`
            : `${diff.split('\n').length} lines`}
        </span>
        {currentFile !== undefined && (
          <span className="flex items-center gap-1.5">
            <StatusSquare
              status={currentFile.viewed ? 'success' : 'neutral'}
              size="w-2 h-2"
              label={currentFile.viewed ? 'VIEWED' : 'NOT VIEWED'}
            />
            <span
              className={currentFile.viewed ? 'text-primary' : 'text-outline'}
            >
              {currentFile.viewed
                ? 'viewed'
                : markViewedKeys === undefined
                  ? 'not viewed'
                  : `press ${markViewedKeys} to mark viewed`}
            </span>
          </span>
        )}
      </div>
      <div className="flex items-center gap-6 font-mono text-[10px] uppercase tracking-widest text-outline">
        {files.length > 0 && (
          <span>
            {viewedCount}/{files.length} viewed
          </span>
        )}
        <span>UTF-8</span>
        <span>LF</span>
        <span>DIFF</span>
      </div>
    </div>
  );
}

interface DiffPageBodyProps {
  isAwaitingHydration: boolean;
  hydrationError: string | null;
  isHydrating: boolean;
  onRetry: () => void;
  showRangeDiff: boolean;
  rangeLoading: boolean;
  rangeError: string | null;
  rangeResult: RangeDiffResult | null;
  fullDiffLoading: boolean;
  fullDiffError: string | null;
  children: ReactNode;
}

export function DiffPageBody({
  isAwaitingHydration,
  hydrationError,
  isHydrating,
  onRetry,
  showRangeDiff,
  rangeLoading,
  rangeError,
  rangeResult,
  fullDiffLoading,
  fullDiffError,
  children,
}: DiffPageBodyProps): React.ReactElement {
  return (
    <div className="flex-1 min-h-0">
      {isAwaitingHydration ? (
        <HydrationState
          error={hydrationError}
          isHydrating={isHydrating}
          onRetry={onRetry}
        />
      ) : showRangeDiff ? (
        rangeLoading ? (
          <LoadingState label="Loading range diff…" />
        ) : rangeError !== null ? (
          <ErrorState message={`Failed to load range diff: ${rangeError}`} />
        ) : rangeResult !== null ? (
          <RangeDiffView result={rangeResult} />
        ) : (
          <LoadingState label="Loading range diff…" />
        )
      ) : fullDiffLoading ? (
        <LoadingState label="Loading diff…" />
      ) : fullDiffError !== null ? (
        <ErrorState message={`Failed to load diff: ${fullDiffError}`} />
      ) : (
        children
      )}
    </div>
  );
}

function HydrationState({
  error,
  isHydrating,
  onRetry,
}: {
  error: string | null;
  isHydrating: boolean;
  onRetry: () => void;
}): React.ReactElement {
  if (isHydrating || error === null) {
    return <LoadingState label="Preparing pull request details…" />;
  }

  return (
    <div className="w-full h-full flex flex-col items-center justify-center gap-3 text-error text-sm">
      <span>Failed to prepare pull request details: {error}</span>
      <button
        type="button"
        onClick={onRetry}
        className="font-mono text-[10px] uppercase tracking-widest border border-error/60 px-3 py-1 text-error hover:bg-error/10"
      >
        Retry hydration
      </button>
    </div>
  );
}

function LoadingState({ label }: { label: string }): React.ReactElement {
  return (
    <div className="w-full h-full flex items-center justify-center text-on-surface-variant font-mono uppercase tracking-widest text-xs">
      {label}
    </div>
  );
}

function ErrorState({ message }: { message: string }): React.ReactElement {
  return (
    <div className="w-full h-full flex items-center justify-center text-error text-sm">
      {message}
    </div>
  );
}
