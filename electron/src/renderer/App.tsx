import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import { AppFooter } from './components/AppFooter';
import { CommandPalette } from './components/CommandPalette';
import { FooterSlotProvider } from './components/FooterSlot';
import { HelpOverlay } from './components/HelpOverlay';
import { RepoPicker } from './components/RepoPicker';
import ToastContainer from './components/ToastContainer';
import { useAppHotkeys } from './hooks/useAppHotkeys';
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
  const repoPickerDialogRef = React.useRef<HTMLDivElement>(null);
  const repoPickerReturnFocusRef = React.useRef<HTMLElement | null>(null);
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

  React.useEffect(() => {
    if (mode !== 'repo-picker') return;

    repoPickerReturnFocusRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;

    const getFocusableElements = (): HTMLElement[] => {
      const dialog = repoPickerDialogRef.current;
      if (!dialog) return [];

      return Array.from(
        dialog.querySelectorAll<HTMLElement>(
          'a[href], button:not([disabled]), input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])',
        ),
      ).filter(
        (el) => el.offsetParent !== null || el === document.activeElement,
      );
    };

    const trapFocus = (event: KeyboardEvent): void => {
      if (event.key !== 'Tab') return;

      const dialog = repoPickerDialogRef.current;
      if (!dialog) return;

      const focusableElements = getFocusableElements();
      if (focusableElements.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }

      const firstElement = focusableElements[0];
      const lastElement = focusableElements[focusableElements.length - 1];
      const activeElement = document.activeElement;

      if (!dialog.contains(activeElement)) {
        event.preventDefault();
        firstElement.focus();
        return;
      }

      if (event.shiftKey && activeElement === firstElement) {
        event.preventDefault();
        lastElement.focus();
      } else if (!event.shiftKey && activeElement === lastElement) {
        event.preventDefault();
        firstElement.focus();
      }
    };

    document.addEventListener('keydown', trapFocus, true);
    return () => document.removeEventListener('keydown', trapFocus, true);
  }, [mode]);

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
        // list | search | filter | help | command | repo-picker all keep the PR
        // list visible underneath any overlay.
        return <PRListPage />;
    }
  };

  const closeRepoPicker = (): void => {
    const returnFocusTo = repoPickerReturnFocusRef.current;
    appStore.setState((s) => ({ ...s, mode: 'list' }));
    window.requestAnimationFrame(() => {
      if (returnFocusTo?.isConnected === true) returnFocusTo.focus();
      repoPickerReturnFocusRef.current = null;
    });
  };

  return (
    <div className="w-full h-screen flex flex-col bg-surface text-on-surface">
      <div className="flex-1 min-h-0 flex flex-col">{renderPage()}</div>
      <AppFooter />
      <CommandPalette />
      {mode === 'repo-picker' && (
        <div
          ref={repoPickerDialogRef}
          role="dialog"
          aria-modal="true"
          aria-label="Repository picker"
          tabIndex={-1}
          className="fixed inset-0 z-40 bg-transparent backdrop-blur-[20px] flex items-start justify-center pt-[153px]"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) closeRepoPicker();
          }}
        >
          <div className="w-full max-w-2xl mx-4 bg-surface-container-lowest border border-primary shadow-[0_10px_30px_rgba(25,28,29,0.04)] overflow-hidden p-4">
            <RepoPicker onClose={closeRepoPicker} />
          </div>
        </div>
      )}
      <HelpOverlay />
      <ToastContainer />
    </div>
  );
}

export function AuthGate(): React.ReactElement {
  useAppHotkeys();
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
