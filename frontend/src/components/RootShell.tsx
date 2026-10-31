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
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { AddRepoDialogOverlay } from './AddRepoDialogOverlay';
import { AppTitleBar } from './AppTitleBar';
import ToastContainer from './ToastContainer';

/** Registers hotkeys. Lives inside OverlayProvider. */
function HotkeyRegistrar(): null {
  useAppHotkeys();
  return null;
}

/**
 * Bridges confirmingAction==='command' → overlays.openCommand().
 * Needs OverlayProvider context (provided by RootShell).
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
  return (
    <div className="flex h-screen flex-col bg-surface text-on-surface">
      <HotkeyRegistrar />
      <ConfirmingActionOverlayBridge />
      <AppTitleBar />
      <AddRepoDialogOverlay />
      <div className="min-h-0 flex-1">
        <Outlet />
      </div>
      <ToastContainer />
    </div>
  );
}

export function RootShell(): React.ReactElement {
  return (
    <OverlayProvider>
      <RootShellInner />
    </OverlayProvider>
  );
}
