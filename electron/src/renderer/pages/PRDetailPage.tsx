import React, { useState, useEffect } from 'react';
import { MainLayout } from '../layouts/MainLayout';
import { DiffViewer } from '../components/DiffViewer';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { usePRList } from '../queries/usePRList';
import { useWebSocket } from '../hooks/useWebSocket';

export function PRDetailPage() {
  const selectedIndex = useStore(appStore, (s) => s.selectedPRIndex);
  const filterState = useStore(appStore, (s) => s.filterState);
  const [diff, setDiff] = useState<string>('');
  const [diffLoading, setDiffLoading] = useState(false);
  const [diffError, setDiffError] = useState<string | null>(null);

  const { data: prs = [], isLoading, error } = usePRList(filterState);
  const pr = prs[selectedIndex];
  const { request } = useWebSocket();

  useEffect(() => {
    if (pr === undefined) {
      return;
    }

    setDiffLoading(true);
    setDiffError(null);

    request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` })
      .then((data) => {
        const payload = data as { diff?: string };
        setDiff(payload.diff ?? '');
        setDiffLoading(false);
      })
      .catch((err: unknown) => {
        const message = err instanceof Error ? err.message : 'Unknown error';
        console.error('Failed to fetch diff:', message);
        setDiffError(message);
        setDiffLoading(false);
      });
  }, [selectedIndex, request]);

  return (
    <MainLayout
      toolbar={
        <div className="flex items-center gap-2">
          <span className="font-mono text-sm text-on-surface-variant">
            PR #{selectedIndex + 1}
          </span>
          {pr !== undefined && (
            <span className="text-sm text-on-surface truncate max-w-xs">
              {pr.title}
            </span>
          )}
        </div>
      }
    >
      <div className="flex flex-col gap-4">
        <div>
          <h2 className="text-lg font-bold text-on-surface mb-2">PR Details</h2>
          {isLoading && (
            <p className="text-sm text-on-surface-variant">Loading PR details...</p>
          )}
          {error !== null && (
            <p className="text-sm text-error">
              Failed to load PR: {(error as Error).message}
            </p>
          )}
          {!isLoading && error === null && pr !== undefined && (
            <p className="text-sm text-on-surface-variant font-mono">
              {pr.author} · {pr.state}
            </p>
          )}
          {!isLoading && error === null && pr === undefined && (
            <p className="text-sm text-on-surface-variant">
              Open PR details sidebar to view review status, comments, and metadata.
            </p>
          )}
        </div>
        {diffLoading && (
          <p className="text-sm text-on-surface-variant">Loading diff...</p>
        )}
        {diffError !== null && (
          <p className="text-sm text-error">Failed to load diff: {diffError}</p>
        )}
        <DiffViewer diff={diff} className="h-96" />
      </div>
    </MainLayout>
  );
}
