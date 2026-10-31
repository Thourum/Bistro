import type React from 'react';
import { useReviewThreads } from '../hooks/useReviewThreads';
import { type ReviewThread, threadLifecycle } from '../types';

interface Props {
  prId: string;
}

/**
 * Counts threads that still block a clean merge: anything in the `open`
 * lifecycle bucket. Resolved and outdated threads do not block; unknown-state
 * threads are NOT counted as blocking (we don't claim a state the backend
 * hasn't confirmed) but they are surfaced separately so the reviewer knows the
 * picture is incomplete.
 */
function summarize(threads: ReviewThread[]): {
  open: number;
  unknown: number;
} {
  let open = 0;
  let unknown = 0;
  for (const t of threads) {
    const lifecycle = threadLifecycle(t);
    if (lifecycle === 'open') open += 1;
    else if (lifecycle === 'unknown') unknown += 1;
  }
  return { open, unknown };
}

/**
 * A compact merge-readiness pill for the PR detail footer slot. Green when no
 * open threads remain, amber with a count when work is outstanding. Mirrors
 * the footer's monospace / uppercase chrome and the status-dot vocabulary used
 * by the sync indicator.
 *
 * Renders nothing while the first fetch is in flight (no flicker) and stays
 * silent on error — the threads view itself owns the loud error surface.
 */
export function MergeReadinessBadge({
  prId,
}: Props): React.ReactElement | null {
  const { threads, loading, error } = useReviewThreads(prId);

  if (loading || error !== null) return null;

  const { open, unknown } = summarize(threads);
  const ready = open === 0;

  return (
    <span
      data-testid="merge-readiness-badge"
      className="flex items-center gap-2 text-[10px] uppercase tracking-widest font-mono"
    >
      <span
        aria-hidden="true"
        className={`w-2 h-2 ${ready ? 'bg-green-500' : 'bg-yellow-500'}`}
      />
      <span className={ready ? 'text-on-surface' : 'text-yellow-600'}>
        {ready
          ? 'Ready to merge'
          : `${open} open thread${open === 1 ? '' : 's'}`}
      </span>
      {unknown > 0 && (
        <span
          className="text-outline"
          title="Thread state could not be confirmed"
        >
          · {unknown} unknown
        </span>
      )}
    </span>
  );
}
