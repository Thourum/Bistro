import { Expertise } from '@bindings/services';
import { useCallback } from 'react';
import { useStore } from '../hooks/useStore';
import { callWrite, faultReason } from '../services/engine/call';
import { handleExpertiseError } from '../services/events/expertise';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { Button } from './ui/button';

interface ExpertiseNudgeProps {
  repoId: string;
}

export function ExpertiseNudge({
  repoId,
}: ExpertiseNudgeProps): React.ReactElement | null {
  const nudge = useStore(
    appStore,
    (s: UIState) => s.expertiseNudges?.[repoId] ?? null,
  );

  const clearNudge = useCallback(() => {
    appStore.setState((s) => {
      const { [repoId]: _removed, ...expertiseNudges } =
        s.expertiseNudges ?? {};
      return { ...s, expertiseNudges };
    });
  }, [repoId]);

  const handleEnable = (): void => {
    void callWrite(() => Expertise.Enable({ repo_id: repoId })).match(
      () => clearNudge(),
      (fault) =>
        handleExpertiseError({
          repo_id: repoId,
          code: faultReason(fault) ?? fault.tag ?? 'internal',
          message: fault.message,
        }),
    );
  };

  const handleDismiss = (): void => {
    void callWrite(() => Expertise.DismissNudge({ repo_id: repoId })).match(
      clearNudge,
      (fault) =>
        handleExpertiseError({
          repo_id: repoId,
          code: faultReason(fault) ?? fault.tag ?? 'internal',
          message: fault.message,
        }),
    );
  };

  if (nudge === null) return null;

  return (
    <div className="border border-primary/30 bg-surface-container-low px-4 py-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex items-center gap-3">
        <span className="material-symbols-outlined text-primary text-[20px]">
          psychology
        </span>
        <p className="font-mono text-[11px] uppercase tracking-widest text-on-surface">
          You've reviewed {nudge.reviewed_count} PRs from this repo. Enable
          expertise indexing?
        </p>
      </div>
      <div className="flex shrink-0 gap-2">
        <Button type="button" size="sm" onClick={handleEnable}>
          Enable
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleDismiss}
        >
          Dismiss
        </Button>
      </div>
    </div>
  );
}
