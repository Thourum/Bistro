import { useStore } from '../hooks/useStore';
import type { UIState } from '../store/ui';
import { appStore } from '../store';

export default function StatusBar() {
  const selectedIndex = useStore(appStore, (state: UIState) => state.selectedPRIndex);

  return (
    <div className="status-bar">
      <div className="status-left">
        <span>Synced at {new Date().toLocaleTimeString()}</span>
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
