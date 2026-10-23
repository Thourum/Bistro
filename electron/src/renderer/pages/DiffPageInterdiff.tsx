import type { AppSettings } from '../../types/settings';
import { DiffView, type DiffViewProps } from '../components/diff/DiffView';
import type { InterdiffResult } from '../types';

interface InterdiffPaneProps {
  result: InterdiffResult | null;
  loading: boolean;
  error: string | null;
  theme: DiffViewProps['theme'];
  diffStyle: AppSettings['diff']['style'];
}

/** The current file between two patchset heads, in place of the PR diff. */
export function InterdiffPane({
  result,
  loading,
  error,
  theme,
  diffStyle,
}: InterdiffPaneProps): React.ReactElement {
  if (error !== null) {
    return (
      <PaneMessage tone="error">Failed to load interdiff: {error}</PaneMessage>
    );
  }
  if (loading || result === null)
    return <PaneMessage>Loading interdiff…</PaneMessage>;
  if (!result.available)
    return <PaneMessage>Interdiff unavailable: {result.reason}</PaneMessage>;
  if (result.patch === '') {
    return (
      <PaneMessage>
        {result.path} is identical in P{result.from_patchset} and P
        {result.to_patchset}
      </PaneMessage>
    );
  }
  return (
    <DiffView
      diff={result.patch}
      theme={theme}
      diffStyle={diffStyle}
      syncFileSelection={false}
    />
  );
}

function PaneMessage({
  tone = 'neutral',
  children,
}: {
  tone?: 'neutral' | 'error';
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <div
      className={`w-full h-full flex items-center justify-center font-mono text-xs ${
        tone === 'error' ? 'text-error' : 'text-on-surface-variant'
      }`}
    >
      {children}
    </div>
  );
}
