import { useStore } from '../hooks/useStore';
import type { UIState } from '../store/ui';
import { appStore } from '../store';

export default function PRDetail() {
  const selectedIndex = useStore(appStore, (state: UIState) => state.selectedPRIndex);

  return (
    <div className="pr-detail-view">
      <div className="detail-header">
        <button onClick={() => appStore.setState((s) => ({ ...s, mode: 'list' }))}>
          ← Back to List
        </button>
        <h1>PR #{selectedIndex + 1} Detail</h1>
      </div>

      <div className="detail-content">
        <div className="diff-viewer-placeholder">
          <p>Diff viewer will render here (Phase 4)</p>
        </div>

        <div className="comments-pane-placeholder">
          <p>Comments will render here (Phase 4)</p>
        </div>
      </div>

      <div className="status-sidebar-placeholder">
        <p>Status sidebar will render here (Phase 4)</p>
      </div>
    </div>
  );
}
