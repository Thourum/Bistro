import { useMemo } from 'react';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import type { CICheck, Comment, PR } from '../types';
import { aggregateCounts } from '../utils/classification';
import { repoIdForPR } from '../utils/expertise';
import { ExternalLink } from './ExternalLink';
import { type StatusKind, StatusSquare } from './StatusSquare';

const REVIEW_STATUS: Record<string, StatusKind> = {
  approved: 'success',
  changes_requested: 'failure',
  pending: 'neutral',
};

const REVIEW_LABELS: Record<string, string> = {
  approved: 'Approved',
  changes_requested: 'Changes requested',
  pending: 'Pending review',
};

const CI_STATUS: Record<string, StatusKind> = {
  success: 'success',
  failure: 'failure',
  pending: 'running',
};

const CI_LABELS: Record<string, string> = {
  success: 'CI SUCCESS',
  failure: 'CI FAILED',
  pending: 'CI RUNNING',
};

function formatIndexedFiles(
  done: number | undefined,
  total: number | undefined,
): string {
  const formattedDone = (done ?? 0).toLocaleString();
  const formattedTotal = (total ?? 0).toLocaleString();
  return `${formattedDone}/${formattedTotal}`;
}

interface StatusSidebarProps {
  pr: PR;
  ciChecks: CICheck[];
  comments?: Comment[];
  touchesYouCount?: number;
}

export default function StatusSidebar({
  pr,
  ciChecks,
  comments = [],
  touchesYouCount,
}: StatusSidebarProps) {
  const counts = useMemo(() => aggregateCounts(comments), [comments]);
  const severitySummary = [
    `Human ${counts.human}`,
    `Critical ${counts.bySeverity.critical}`,
    `Warning ${counts.bySeverity.warning}`,
    `Nit ${counts.bySeverity.nit}`,
  ].join(' · ');
  const vendorSummary = [
    `CodeRabbit ${counts.byVendor.coderabbit ?? 0}`,
    `Greptile ${counts.byVendor.greptile ?? 0}`,
    `Other ${counts.byVendor.other ?? 0}`,
  ].join(' · ');
  const reviewStatus = pr.review_status ?? 'pending';
  const reviewKind = REVIEW_STATUS[reviewStatus] ?? 'neutral';
  const reviewLabel = REVIEW_LABELS[reviewStatus] ?? reviewStatus;
  const expertiseStatus = useStore(
    appStore,
    (s: UIState) => s.expertiseRepoStatuses?.[repoIdForPR(pr)],
  );

  return (
    <div className="w-56 flex-shrink-0 border-l border-outline-variant bg-surface-container overflow-y-auto p-4 space-y-5 text-sm">
      {/* Review Status */}
      <section>
        <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-outline mb-2">
          Review
        </h3>
        <p className="flex items-center gap-2 font-medium text-on-surface">
          <StatusSquare status={reviewKind} label={reviewLabel} />
          {reviewLabel}
        </p>
        {pr.approved_by !== null && (
          <p className="mt-1 font-mono text-[10px] text-outline">
            by {pr.approved_by}
          </p>
        )}
        {pr.changes_requested_by !== null && (
          <p className="mt-1 font-mono text-[10px] text-outline">
            by {pr.changes_requested_by}
          </p>
        )}
      </section>

      {/* PR Meta */}
      <section>
        <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-outline mb-2">
          Details
        </h3>
        <div className="space-y-1 font-mono text-[11px] text-outline">
          <p>
            {pr.comment_count} comment{pr.comment_count !== 1 ? 's' : ''}
          </p>
          <p>Updated {new Date(pr.updated_at).toLocaleDateString()}</p>
        </div>
      </section>

      {/* CI Checks */}
      <section>
        <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-outline mb-2">
          CI Checks
        </h3>
        {ciChecks.length === 0 ? (
          <p className="font-mono text-[11px] text-outline">No CI checks</p>
        ) : (
          <ul className="space-y-1.5">
            {ciChecks.map((check) => (
              <li
                key={check.name}
                className="flex items-center gap-2 font-mono text-[11px]"
              >
                <StatusSquare
                  status={CI_STATUS[check.state] ?? 'neutral'}
                  label={CI_LABELS[check.state] ?? 'CI UNKNOWN'}
                />
                {check.url != null ? (
                  <ExternalLink
                    href={check.url}
                    className="text-primary hover:underline truncate"
                    aria-label={check.name}
                  >
                    {check.name}
                  </ExternalLink>
                ) : (
                  <span className="text-on-surface truncate">{check.name}</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>

      {/* Comment Classification */}
      <section>
        <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-outline mb-2">
          Comments
        </h3>
        <div className="space-y-1 font-mono text-[11px] text-outline">
          <p>{severitySummary}</p>
          <p>{vendorSummary}</p>
        </div>
      </section>

      {/* Expertise */}
      <section>
        <h3 className="font-mono text-[10px] font-bold uppercase tracking-widest text-outline mb-2">
          Expertise
        </h3>
        <div className="space-y-1 font-mono text-[11px] text-outline">
          {touchesYouCount !== undefined && (
            <p>Touches Your Code {touchesYouCount}</p>
          )}
          <p>
            {pr.touches_you ? 'Touches your code' : 'No local expertise match'}
          </p>
          {expertiseStatus?.status === 'indexing' && (
            <p>
              indexing{' '}
              {formatIndexedFiles(
                expertiseStatus.files_done,
                expertiseStatus.files_total,
              )}
            </p>
          )}
          {expertiseStatus?.status === 'error' && (
            <p className="text-error">
              error:{' '}
              {expertiseStatus.message ?? expertiseStatus.code ?? 'unknown'}
            </p>
          )}
        </div>
      </section>
    </div>
  );
}
