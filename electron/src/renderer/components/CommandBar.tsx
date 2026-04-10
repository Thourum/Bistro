import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

export default function CommandBar() {
  const mode = useStore(appStore, (state: UIState) => state.mode);

  return (
    <div className="command-bar">
      <div className="mode-indicator">
        {mode === 'login' && <span>Login</span>}
        {mode === 'list' && <span>List Mode</span>}
        {mode === 'search' && <span>Search Mode</span>}
        {mode === 'detail' && <span>Detail Mode</span>}
        {mode === 'help' && <span>Help Mode</span>}
        {mode === 'command' && <span>Command Mode</span>}
        {mode === 'filter' && <span>Filter Mode</span>}
        {mode === 'settings' && <span>Settings Mode</span>}
      </div>
    </div>
  );
}
