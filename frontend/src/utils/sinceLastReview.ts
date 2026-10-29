import { splitUnifiedDiff } from '../components/diff/diffPathUtils';
import type { TriageBaselineSource, TriageResult } from '../types';

const MINUTE_MS = 60_000;
const HOUR_MS = 60 * MINUTE_MS;
const DAY_MS = 24 * HOUR_MS;

const BASELINE_LABELS = {
  github_review: 'your review',
  bistro_action: 'your review',
  all_viewed: 'all viewed',
} satisfies Record<TriageBaselineSource, string>;

/** Status-bar line for a triage result, e.g. `since P3 (your review, 2d ago): 4 of 37 files changed`. */
export function triageSummary(result: TriageResult, now: Date): string {
  if (result.state !== 'ok') return result.reason;
  const counts = `${result.changed_count} of ${result.total_count} files changed`;
  if (result.baseline === null) {
    return `P${result.from_patchset} → P${result.to_patchset}: ${counts}`;
  }
  const label = BASELINE_LABELS[result.baseline.source];
  const age = relativeAge(new Date(result.baseline.reviewed_at), now);
  return `since P${result.from_patchset} (${label}, ${age}): ${counts}`;
}

/** Keeps only the files of a unified diff whose path is in `paths`. */
export function filterDiffToPaths(
  diff: string,
  paths: ReadonlySet<string>,
): string {
  return splitUnifiedDiff(diff)
    .filter((file) => paths.has(file.path))
    .map((file) => file.patch)
    .join('\n');
}

function relativeAge(then: Date, now: Date): string {
  const elapsed = Math.max(0, now.getTime() - then.getTime());
  if (elapsed < MINUTE_MS) return 'just now';
  if (elapsed < HOUR_MS) return `${Math.floor(elapsed / MINUTE_MS)}m ago`;
  if (elapsed < DAY_MS) return `${Math.floor(elapsed / HOUR_MS)}h ago`;
  return `${Math.floor(elapsed / DAY_MS)}d ago`;
}
