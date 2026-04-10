import { useEffect } from 'react';
import type { ActionResult, PendingAction } from '../types';

const ACTION_LABELS: Record<string, string> = {
  approve: 'Approving',
  request_changes: 'Requesting changes',
  post_comment: 'Posting comment',
};

const AUTO_DISMISS_MS = 3000;

interface ActionFeedbackProps {
  pendingAction: PendingAction | null;
  actionResult: ActionResult | null;
  onDismiss: () => void;
  onRetry?: () => void;
}

export default function ActionFeedback({
  pendingAction,
  actionResult,
  onDismiss,
  onRetry,
}: ActionFeedbackProps) {
  // Auto-dismiss successful results after timeout
  useEffect(() => {
    if (actionResult?.success) {
      const timer = setTimeout(onDismiss, AUTO_DISMISS_MS);
      return () => clearTimeout(timer);
    }
    return undefined;
  }, [actionResult, onDismiss]);

  if (pendingAction !== null) {
    const label = ACTION_LABELS[pendingAction.type] ?? pendingAction.type;
    return (
      <div className="px-4 py-2 bg-surface-container border-b border-outline-variant text-sm flex items-center gap-2">
        <span className="animate-spin text-primary">⟳</span>
        <span className="text-on-surface-variant">{label}...</span>
      </div>
    );
  }

  if (actionResult !== null) {
    const colorClass = actionResult.success
      ? 'text-green-600 dark:text-green-400'
      : 'text-error';
    const icon = actionResult.success ? '✓' : '✗';
    return (
      <div className="px-4 py-2 bg-surface-container border-b border-outline-variant text-sm flex items-center gap-2">
        <span className={colorClass}>{icon}</span>
        <span className={colorClass}>{actionResult.message}</span>
        {!actionResult.success && (
          <span className="ml-auto flex items-center gap-2">
            {onRetry != null && (
              <button
                onClick={onRetry}
                className="text-xs font-medium text-primary hover:underline"
                aria-label="Retry"
              >
                Retry
              </button>
            )}
            <button
              onClick={onDismiss}
              className="text-xs text-on-surface-variant hover:text-on-surface"
            >
              Dismiss
            </button>
          </span>
        )}
      </div>
    );
  }

  return null;
}
