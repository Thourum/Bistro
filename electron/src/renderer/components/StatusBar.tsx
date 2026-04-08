import { useStore } from '../hooks/useStore';
import type { UIState } from '../store/ui';
import { appStore } from '../store';

const MODE_HINTS: Record<UIState['mode'], string> = {
  list: 'j/k nav | Enter open | / search | f filter | : cmd | ? help',
  detail: 'hjkl scroll | n/p files | aa approve | c comment | Esc back',
  search: 'Type to search | Enter apply | Esc cancel',
  help: 'Esc close',
  command: 'Type command | Enter execute | Esc cancel',
  filter: 'o open | d draft | a all | f/Esc close',
  settings: 'Esc back',
};

export default function StatusBar() {
  const mode = useStore(appStore, (state: UIState) => state.mode);
  const syncStatus = useStore(appStore, (state: UIState) => state.syncStatus);
  const lastSyncAt = useStore(appStore, (state: UIState) => state.lastSyncAt);
  const syncError = useStore(appStore, (state: UIState) => state.syncError);
  const confirmingAction = useStore(appStore, (state: UIState) => state.confirmingAction);

  const syncLabel = (() => {
    if (syncStatus === 'ready' && lastSyncAt !== null) {
      return `Synced at ${new Date(lastSyncAt).toLocaleTimeString()}`;
    }
    if (syncStatus === 'error' && syncError !== null) {
      return `Error: ${syncError}`;
    }
    return syncStatus.charAt(0).toUpperCase() + syncStatus.slice(1);
  })();

  const hint =
    confirmingAction === 'approve'
      ? "Press 'a' again to confirm approval"
      : MODE_HINTS[mode] ?? '';

  return (
    <div className="flex items-center justify-between px-4 py-1.5 bg-surface-container border-t border-outline-variant text-[11px] font-mono">
      <span className="text-on-surface-variant">{hint}</span>
      <span>{syncLabel}</span>
    </div>
  );
}
