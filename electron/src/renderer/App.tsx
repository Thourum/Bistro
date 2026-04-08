import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { appStore } from './store';
import { useStore } from './hooks/useStore';
import { useKeyboardIntegration } from './hooks/useKeyboardIntegration';
import type { UIState } from './store/ui';
import { LoginPage } from './pages/LoginPage';
import { PRListPage } from './pages/PRListPage';
import { PRDetailPage } from './pages/PRDetailPage';
import { SettingsPage } from './pages/SettingsPage';
import { CommandPalette } from './components/CommandPalette';
import { HelpOverlay } from './components/HelpOverlay';
import './App.css';

const queryClient = new QueryClient();

export function AppContent(): React.ReactElement {
  useKeyboardIntegration();

  const mode = useStore(appStore, (state: UIState) => state.mode);
  const confirmingAction = useStore(appStore, (state: UIState) => state.confirmingAction);

  // Command palette is driven by confirmingAction === 'command'.
  // Map it to the 'command' mode so CommandPalette (which reads mode) opens.
  React.useEffect(() => {
    if (confirmingAction === 'command') {
      appStore.setState((s) => ({ ...s, mode: 'command', confirmingAction: null }));
    }
  }, [confirmingAction]);

  const renderPage = (): React.ReactElement => {
    switch (mode) {
      case 'detail':
        return <PRDetailPage />;
      case 'settings':
        return <SettingsPage />;
      default:
        // list | search | filter | help | command all keep the PR list visible
        // underneath any overlay.
        return <PRListPage />;
    }
  };

  return (
    <div className="w-full h-screen bg-surface-container-lowest">
      {renderPage()}
      <CommandPalette />
      <HelpOverlay />
    </div>
  );
}

export function AuthGate(): React.ReactElement {
  // LoginPage is shown when there are no connected providers.
  // For now we render it as a standalone route at the 'login' confirmingAction.
  // Full OAuth flow integration is handled in a subsequent task.
  return <LoginPage />;
}

export default function App(): React.ReactElement {
  return (
    <QueryClientProvider client={queryClient}>
      <AppContent />
    </QueryClientProvider>
  );
}
