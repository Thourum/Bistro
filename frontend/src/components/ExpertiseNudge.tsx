import { Fault } from '@thourum/fault';
import { useCallback } from 'react';
import { useStore } from '../hooks/useStore';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { addToast } from './ToastContainer';
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
    void wsClient
      .request('EXPERTISE_ENABLE', { repo_id: repoId })
      .mapErr((cause) =>
        new Fault(cause).withTag('CONNECTION_ERROR').withCause(cause),
      )
      .match(
        () => clearNudge(),
        (err) => {
          addToast({
            type: 'error',
            message: `Enabling expertise failed: ${err.message}`,
            dismissible: true,
          });
        },
      );
  };

  const handleDismiss = (): void => {
    wsClient.send('EXPERTISE_NUDGE_DISMISS', { repo_id: repoId });
    clearNudge();
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
