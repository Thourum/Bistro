import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { appStore } from './store';
import { useStore } from './hooks/useStore';
import type { UIState } from './store/ui';
import PRList from './pages/PRList';
import PRDetail from './pages/PRDetail';
import './App.css';

const queryClient = new QueryClient();

function AppContent() {
  const mode = useStore(appStore, (state: UIState) => state.mode);

  return (
    <div className="app">
      {mode === 'list' && <PRList />}
      {mode === 'detail' && <PRDetail />}
    </div>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AppContent />
    </QueryClientProvider>
  );
}
