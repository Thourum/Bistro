import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import { AppFooter } from './components/AppFooter';
import { CommandPalette } from './components/CommandPalette';
import { FooterSlotProvider } from './components/FooterSlot';
import { HelpOverlay } from './components/HelpOverlay';
import ToastContainer from './components/ToastContainer';
import { useKeyboardIntegration } from './hooks/useKeyboardIntegration';
import { useSettingsEffect } from './hooks/useSettingsEffect';
import { useStore } from './hooks/useStore';
import { useWebSocket } from './hooks/useWebSocket';
import { DiffPage } from './pages/DiffPage';
import { LoginPage } from './pages/LoginPage';
import { OnboardingPage } from './pages/OnboardingPage';
import { PRDetailPage } from './pages/PRDetailPage';
import { PRListPage } from './pages/PRListPage';
import { SettingsPage } from './pages/SettingsPage';
import { appStore } from './store';
import { initSettings, settingsStore } from './store/settings';
import type { UIState } from './store/ui';

const queryClient = new QueryClient();

export function AppContent(): React.ReactElement {
  const mode = useStore(appStore, (state: UIState) => state.mode);
  const confirmingAction = useStore(
    appStore,
    (state: UIState) => state.confirmingAction,
  );

  // Command palette is driven by confirmingAction === 'command'.
  // Map it to the 'command' mode so CommandPalette (which reads mode) opens.
  React.useEffect(() => {
    if (confirmingAction === 'command') {
      appStore.setState((s) => ({
        ...s,
        mode: 'command',
        confirmingAction: null,
      }));
    }
  }, [confirmingAction]);

  const renderPage = (): React.ReactElement => {
    switch (mode) {
      case 'onboarding':
        return <OnboardingPage />;
      case 'detail':
        return <PRDetailPage />;
      case 'diff':
        return <DiffPage />;
      case 'settings':
        return <SettingsPage />;
      default:
        // list | search | filter | help | command all keep the PR list visible
        // underneath any overlay.
        return <PRListPage />;
    }
  };

  return (
    <div className="w-full h-screen flex flex-col bg-surface text-on-surface">
      <div className="flex-1 min-h-0 flex flex-col">{renderPage()}</div>
      <AppFooter />
      <CommandPalette />
      <HelpOverlay />
      <ToastContainer />
    </div>
  );
}

export function AuthGate(): React.ReactElement {
  useKeyboardIntegration();
  useSettingsEffect();
  useWebSocket();

  const isAuthenticated = useStore(
    appStore,
    (state: UIState) => state.isAuthenticated,
  );
  const authChecked = useStore(appStore, (state: UIState) => state.authChecked);
  const onboardingComplete = useStore(
    settingsStore,
    (s) => s.appearance.onboardingComplete,
  );

  // Keep mode in sync with auth state so the centralized keyboard system
  // activates the correct bindings on every page, including login.
  React.useEffect(() => {
    if (!isAuthenticated) {
      appStore.setState((s) => ({ ...s, mode: 'login' }));
    } else if (appStore.state.mode === 'login') {
      appStore.setState((s) => ({
        ...s,
        mode: onboardingComplete ? 'list' : 'onboarding',
      }));
    }
  }, [isAuthenticated, onboardingComplete]);

  if (!authChecked) {
    return (
      <div className="w-full h-screen bg-surface flex items-center justify-center">
        <div className="text-center space-y-4">
          <h1 className="text-2xl font-black tracking-tighter text-on-surface uppercase">
            BISTRO
          </h1>
          <p className="text-sm text-on-surface-variant font-mono animate-pulse uppercase tracking-widest">
            Connecting...
          </p>
        </div>
      </div>
    );
  }

  if (!isAuthenticated) {
    return (
      <div className="w-full h-screen flex flex-col bg-surface text-on-surface">
        <div className="flex-1 min-h-0 flex flex-col">
          <LoginPage />
        </div>
        <AppFooter />
        <ToastContainer />
      </div>
    );
  }

  return <AppContent />;
}

export default function App(): React.ReactElement {
  const [ready, setReady] = React.useState(false);

  React.useEffect(() => {
    void initSettings().finally(() => setReady(true));
  }, []);

  if (!ready) {
    return (
      <div className="w-full h-screen bg-surface flex items-center justify-center">
        <p className="text-sm text-on-surface-variant font-mono uppercase tracking-widest">
          Initialising...
        </p>
      </div>
    );
  }

  return (
    <QueryClientProvider client={queryClient}>
      <FooterSlotProvider>
        <AuthGate />
      </FooterSlotProvider>
    </QueryClientProvider>
  );
}
