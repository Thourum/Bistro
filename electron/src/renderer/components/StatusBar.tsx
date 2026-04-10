import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

const MODE_HINTS: Record<UIState['mode'], string> = {
  login: 'G github | L gitlab | T gitea',
  list: 'j/k nav | Enter open | / search | f filter | : cmd | ? help',
  detail: 'hjkl scroll | n/p files | aa approve | c comment | Esc back',
  search: 'Type to search | Enter apply | Esc cancel',
  help: 'Esc close',
  command: 'Type command | Enter execute | Esc cancel',
  filter: 'o open | d draft | a all | f/Esc close',
  settings: 'Esc back',
};

const STATUS_COLORS: Record<string, string> = {
  connected: 'bg-green-500',
  ready: 'bg-green-500',
  connecting: 'bg-yellow-500',
  disconnected: 'bg-gray-500',
  error: 'bg-red-500',
  rate_limited: 'bg-orange-500',
};

export default function StatusBar() {
  const mode = useStore(appStore, (state: UIState) => state.mode);
  const syncStatus = useStore(appStore, (state: UIState) => state.syncStatus);
  const lastSyncAt = useStore(appStore, (state: UIState) => state.lastSyncAt);
  const syncError = useStore(appStore, (state: UIState) => state.syncError);
  const confirmingAction = useStore(
    appStore,
    (state: UIState) => state.confirmingAction,
  );
  const isOffline = useStore(appStore, (state: UIState) => state.isOffline);

  const syncLabel = (() => {
    if (isOffline) return 'Offline — Using cached data';
    if (syncStatus === 'ready' && lastSyncAt !== null) {
      return `Synced at ${new Date(lastSyncAt).toLocaleTimeString()}`;
    }
    if (syncStatus === 'rate_limited') {
      const suffix = syncError !== null ? ` — ${syncError}` : '';
      return `Rate limited${suffix}`;
    }
    if (syncStatus === 'error' && syncError !== null) {
      return `Error: ${syncError}`;
    }
    return syncStatus.charAt(0).toUpperCase() + syncStatus.slice(1);
  })();

  const dotColor = isOffline
    ? 'bg-gray-500'
    : (STATUS_COLORS[syncStatus] ?? 'bg-gray-500');

  const hint =
    confirmingAction === 'approve'
      ? "Press 'a' again to confirm approval"
      : (MODE_HINTS[mode] ?? '');

  return (
    <div className="flex items-center justify-between px-4 py-1.5 bg-surface-container border-t border-outline-variant text-[11px] font-mono">
      <span className="text-on-surface-variant">{hint}</span>
      <span className="flex items-center gap-1.5">
        <span className={`inline-block w-2 h-2 rounded-full ${dotColor}`} />
        <span>{syncLabel}</span>
      </span>
    </div>
  );
}
