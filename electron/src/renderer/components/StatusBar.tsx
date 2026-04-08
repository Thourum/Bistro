import { useStore } from '../hooks/useStore';
import type { UIState } from '../store/ui';
import { appStore } from '../store';

export default function StatusBar() {
  const selectedIndex = useStore(appStore, (state: UIState) => state.selectedPRIndex);
  const syncStatus = useStore(appStore, (state: UIState) => state.syncStatus);
  const lastSyncAt = useStore(appStore, (state: UIState) => state.lastSyncAt);
  const syncError = useStore(appStore, (state: UIState) => state.syncError);

  const syncLabel = (() => {
    if (syncStatus === 'ready' && lastSyncAt !== null) {
      return `Synced at ${new Date(lastSyncAt).toLocaleTimeString()}`;
    }
    if (syncStatus === 'error' && syncError !== null) {
      return `Error: ${syncError}`;
    }
    return syncStatus.charAt(0).toUpperCase() + syncStatus.slice(1);
  })();

  return (
    <div className="status-bar">
      <div className="status-left">
        <span>{syncLabel}</span>
      </div>
      <div className="status-center">
        <span>j/k to navigate</span>
        <span className="separator">•</span>
        <span>Enter to open</span>
        <span className="separator">•</span>
        <span>? for help</span>
      </div>
      <div className="status-right">
        <span>PR #{selectedIndex + 1}</span>
      </div>
    </div>
  );
}
