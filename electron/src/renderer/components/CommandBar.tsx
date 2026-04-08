import { useState } from 'react';
import { useStore } from '../hooks/useStore';
import type { UIState } from '../store/ui';
import { appStore } from '../store';

export default function CommandBar() {
  const [search, setSearch] = useState('');
  const mode = useStore(appStore, (state: UIState) => state.mode);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSearch(e.currentTarget.value);
  };

  return (
    <div className="command-bar">
      <input
        type="text"
        placeholder="Search by title, author, file, repo..."
        value={search}
        onChange={handleInputChange}
        className="search-input"
      />
      <div className="mode-indicator">
        {mode === 'list' && <span>List Mode</span>}
        {mode === 'search' && <span>Search Mode</span>}
        {mode === 'detail' && <span>Detail Mode</span>}
        {mode === 'help' && <span>Help Mode</span>}
      </div>
    </div>
  );
}
