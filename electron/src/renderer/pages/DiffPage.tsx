import { useEffect, useState } from 'react';
import { PierreDiffView } from '../components/PierreDiffView';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { useWebSocketPRList as usePRList } from '../hooks/useWebSocketPRList';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { UIState } from '../store/ui';
import { createLogger } from '../utils/logger';

const log = createLogger('DiffPage');

/**
 * Full-screen diff view — matches `diff_view_minimal_light/code.html`.
 *
 * Layout: `h-screen` flex column with:
 *   - top bar (PR id / merge path / find input, h-10)
 *   - 25/75 split (file navigator / diff canvas)
 *   - LOC / UTF-8 / CRLF footer
 *
 * Diff rendering is delegated to {@link PierreDiffView} which wraps
 * `@pierre/diffs/react`. Inline comments are disabled this pass.
 */
export function DiffPage() {
  const selectedIndex = useStore(appStore, (s: UIState) => s.selectedPRIndex);
  const filterState = useStore(appStore, (s: UIState) => s.filterState);
  const theme = useStore(settingsStore, (s) => s.appearance.theme);

  const { data: prs = [] } = usePRList(filterState);
  const pr = prs[selectedIndex];
  const { request } = useWebSocket();

  const [diff, setDiff] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [diffError, setDiffError] = useState<string | null>(null);
  const [findQuery, setFindQuery] = useState('');

  useEffect(() => {
    if (pr === undefined) return;
    setLoading(true);
    setDiffError(null);
    request('REQUEST_DIFF', { pr_id: pr.id, repo: `${pr.owner}/${pr.repo}` })
      .then((data) => {
        const payload = data as { diff?: string };
        setDiff(payload.diff ?? '');
        setLoading(false);
      })
      .catch((err: unknown) => {
        const msg = err instanceof Error ? err.message : 'Unknown error';
        log.error({ err: msg }, 'failed to fetch diff');
        setDiffError(msg);
        setLoading(false);
      });
  }, [pr, request]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (appStore.state.mode !== 'diff') return;
      if (e.key === 'Escape') {
        e.preventDefault();
        appStore.setState((s) => ({ ...s, mode: 'detail' }));
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  if (pr === undefined) {
    return (
      <div className="flex-1 flex items-center justify-center text-error">
        PR not found
      </div>
    );
  }

  return (
    <main className="flex-1 flex flex-col overflow-hidden bg-surface text-on-surface">
      {/* Top bar */}
      <div className="shrink-0 h-10 flex items-center justify-between px-4 border-b border-outline-variant/30 bg-surface-container-lowest">
        <div className="flex items-center gap-4 min-w-0">
          <button
            type="button"
            onClick={() => appStore.setState((s) => ({ ...s, mode: 'detail' }))}
            className="font-mono text-[10px] uppercase tracking-widest text-outline hover:text-on-surface transition-colors"
          >
            ← Back
          </button>
          <span className="font-mono text-[11px] font-bold tracking-widest text-primary">
            PR-{pr.number}
          </span>
          <span className="font-mono text-[10px] text-outline truncate">
            {pr.head_branch}
            <span className="text-outline-variant"> → </span>
            {pr.base_branch}
          </span>
        </div>
        <div className="flex items-center gap-3">
          <input
            type="text"
            value={findQuery}
            onChange={(e) => setFindQuery(e.target.value)}
            placeholder="Find in diff…"
            className="font-mono text-[11px] px-3 py-1 w-64 bg-surface border border-outline-variant/40 placeholder:text-outline-variant"
          />
          <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
            ESC to exit
          </span>
        </div>
      </div>

      {/* Body */}
      <div className="flex-1 min-h-0">
        {loading ? (
          <div className="w-full h-full flex items-center justify-center text-on-surface-variant font-mono uppercase tracking-widest text-xs">
            Loading diff…
          </div>
        ) : diffError !== null ? (
          <div className="w-full h-full flex items-center justify-center text-error text-sm">
            Failed to load diff: {diffError}
          </div>
        ) : (
          <PierreDiffView diff={diff} theme={theme} findQuery={findQuery} />
        )}
      </div>

      {/* Status bar (LOC / encoding) */}
      <div className="shrink-0 h-6 px-4 flex items-center justify-between border-t border-outline-variant/30 bg-surface-container-low">
        <div className="font-mono text-[10px] uppercase tracking-widest text-outline">
          {diff.split('\n').length} lines
        </div>
        <div className="flex items-center gap-6 font-mono text-[10px] uppercase tracking-widest text-outline">
          <span>UTF-8</span>
          <span>LF</span>
          <span>DIFF</span>
        </div>
      </div>
    </main>
  );
}
