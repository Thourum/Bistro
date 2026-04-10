import type { CICheck, PR } from '../types';

const REVIEW_COLORS: Record<string, string> = {
  approved: 'text-green-600 dark:text-green-400',
  changes_requested: 'text-error',
  pending: 'text-on-surface-variant',
};

const REVIEW_LABELS: Record<string, string> = {
  approved: 'Approved',
  changes_requested: 'Changes requested',
  pending: 'Pending review',
};

const CI_ICONS: Record<string, string> = {
  success: '✓',
  failure: '✗',
  pending: '⟳',
};

const CI_COLORS: Record<string, string> = {
  success: 'text-green-600 dark:text-green-400',
  failure: 'text-error',
  pending: 'text-on-surface-variant',
};

interface StatusSidebarProps {
  pr: PR;
  ciChecks: CICheck[];
}

export default function StatusSidebar({ pr, ciChecks }: StatusSidebarProps) {
  const reviewStatus = pr.review_status ?? 'pending';
  const reviewColorClass =
    REVIEW_COLORS[reviewStatus] ?? 'text-on-surface-variant';
  const reviewLabel = REVIEW_LABELS[reviewStatus] ?? reviewStatus;

  return (
    <div className="w-56 flex-shrink-0 border-l border-outline-variant bg-surface-container overflow-y-auto p-3 space-y-4 text-sm">
      {/* Review Status */}
      <section>
        <h3 className="text-xs font-bold text-on-surface-variant uppercase mb-2">
          Review
        </h3>
        <p className={`font-medium ${reviewColorClass}`}>{reviewLabel}</p>
        {pr.approved_by !== null && (
          <p className="text-xs text-on-surface-variant mt-1">
            by {pr.approved_by}
          </p>
        )}
        {pr.changes_requested_by !== null && (
          <p className="text-xs text-on-surface-variant mt-1">
            by {pr.changes_requested_by}
          </p>
        )}
      </section>

      {/* PR Meta */}
      <section>
        <h3 className="text-xs font-bold text-on-surface-variant uppercase mb-2">
          Details
        </h3>
        <div className="space-y-1 text-xs text-on-surface-variant">
          <p>
            {pr.comment_count} comment{pr.comment_count !== 1 ? 's' : ''}
          </p>
          <p>Updated {new Date(pr.updated_at).toLocaleDateString()}</p>
        </div>
      </section>

      {/* CI Checks */}
      <section>
        <h3 className="text-xs font-bold text-on-surface-variant uppercase mb-2">
          CI Checks
        </h3>
        {ciChecks.length === 0 ? (
          <p className="text-xs text-on-surface-variant">No CI checks</p>
        ) : (
          <ul className="space-y-1">
            {ciChecks.map((check) => (
              <li key={check.name} className="flex items-center gap-2 text-xs">
                <span
                  className={
                    CI_COLORS[check.state] ?? 'text-on-surface-variant'
                  }
                >
                  {CI_ICONS[check.state] ?? '?'}
                </span>
                {check.url != null ? (
                  <a
                    href={check.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:underline truncate"
                  >
                    {check.name}
                  </a>
                ) : (
                  <span className="text-on-surface">{check.name}</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
