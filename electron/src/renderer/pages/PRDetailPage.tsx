import React from 'react';
import { MainLayout } from '../layouts/MainLayout';
import { DiffViewer } from '../components/DiffViewer';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { usePRList } from '../queries/usePRList';

export function PRDetailPage() {
  const selectedIndex = useStore(appStore, (s) => s.selectedPRIndex);
  const filterState = useStore(appStore, (s) => s.filterState);

  // TODO: Replace with usePRDetail hook once backend diff endpoint is available
  const { data: prs = [], isLoading, error } = usePRList('github:default', filterState);
  const pr = prs[selectedIndex];

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
        {/* TODO: Replace empty string with real diff from backend */}
        <DiffViewer diff="" className="h-96" />
      </div>
    </MainLayout>
  );
}
