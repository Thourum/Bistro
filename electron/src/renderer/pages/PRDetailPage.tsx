import React from 'react';
import { MainLayout } from '../layouts/MainLayout';
import { DiffViewer } from '../components/DiffViewer';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';

export function PRDetailPage() {
  const selectedIndex = useStore(appStore, (s) => s.selectedPRIndex);

  // TODO: Fetch real PR diff from store/query once backend integration is complete
  const sampleDiff = '';

  return (
    <MainLayout
      toolbar={
        <div className="flex items-center gap-2">
          <span className="font-mono text-sm text-on-surface-variant">
            PR #{selectedIndex}
          </span>
        </div>
      }
    >
      <div className="flex flex-col gap-4">
        <div>
          <h2 className="text-lg font-bold text-on-surface mb-2">PR Details</h2>
          <p className="text-sm text-on-surface-variant">
            Open PR details sidebar to view review status, comments, and metadata.
          </p>
        </div>
        <DiffViewer diff={sampleDiff} className="h-96" />
      </div>
    </MainLayout>
  );
}
