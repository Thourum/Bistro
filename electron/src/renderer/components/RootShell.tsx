// RootShell
//
// Root route component shared by ALL routes (login, onboarding, authed).
// Owns: OverlayProvider, HotkeyRegistrar, ConfirmingActionOverlayBridge,
//       ToastContainer.
//
// Moving these here means login hotkeys fire, toasts appear, and the overlay
// system is alive on every route — not just under the authed layout.

import { Outlet } from '@tanstack/react-router';
import React from 'react';
import { useAppHotkeys } from '../hooks/useAppHotkeys';
import { useStore } from '../hooks/useStore';
import { OverlayProvider, useOverlays } from '../overlays/OverlayContext';
import { initEngineStatus } from '../services/engine';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { AddRepoDialogOverlay } from './AddRepoDialogOverlay';
import { AppTitleBar } from './AppTitleBar';
import { EngineCrashedScreen } from './EngineCrashedScreen';
import ToastContainer from './ToastContainer';

/** Registers hotkeys. Lives inside OverlayProvider. */
function HotkeyRegistrar(): null {
  useAppHotkeys();
  return null;
}

/**
 * Bridges confirmingAction==='command' → overlays.openCommand() without
 * writing to mode. Needs OverlayProvider context (provided by RootShell).
 */
function ConfirmingActionOverlayBridge(): null {
  const confirmingAction = useStore(
    appStore,
    (s: UIState) => s.confirmingAction,
  );
  const { openCommand } = useOverlays();

  React.useEffect(() => {
    if (confirmingAction === 'command') {
      appStore.setState((s) => ({ ...s, confirmingAction: null }));
      openCommand();
    }
  }, [confirmingAction, openCommand]);

  return null;
}

function RootShellInner(): React.ReactElement {
  const engineStatus = useStore(appStore, (s: UIState) => s.engineStatus);
  return (
    <div className="flex h-screen flex-col bg-surface text-on-surface">
      <HotkeyRegistrar />
      <ConfirmingActionOverlayBridge />
      <AppTitleBar />
      <AddRepoDialogOverlay />
      <div className="min-h-0 flex-1">
        {engineStatus?.state === 'crashed' ? (
          <EngineCrashedScreen />
        ) : (
          <Outlet />
        )}
      </div>
      <ToastContainer />
    </div>
  );
}

export function RootShell(): React.ReactElement {
  // Main-process status; independent of the WS so crash actions work while it is down.
  React.useEffect(() => initEngineStatus(), []);
  return (
    <OverlayProvider>
      <RootShellInner />
    </OverlayProvider>
  );
}
