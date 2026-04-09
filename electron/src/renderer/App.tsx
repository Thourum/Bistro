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
import ToastContainer from './components/ToastContainer';
import './App.css';

const queryClient = new QueryClient();

export function AppContent(): React.ReactElement {
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
      <ToastContainer />
    </div>
  );
}

export function AuthGate(): React.ReactElement {
  useKeyboardIntegration();

  const isAuthenticated = useStore(appStore, (state: UIState) => state.isAuthenticated);

  // Keep mode in sync with auth state so the centralized keyboard system
  // activates the correct bindings on every page, including login.
  React.useEffect(() => {
    if (!isAuthenticated) {
      appStore.setState((s) => ({ ...s, mode: 'login' }));
    } else if (appStore.state.mode === 'login') {
      appStore.setState((s) => ({ ...s, mode: 'list' }));
    }
  }, [isAuthenticated]);

  if (!isAuthenticated) {
    return <LoginPage />;
  }

  return <AppContent />;
}

export default function App(): React.ReactElement {
  return (
    <QueryClientProvider client={queryClient}>
      <AuthGate />
    </QueryClientProvider>
  );
}
