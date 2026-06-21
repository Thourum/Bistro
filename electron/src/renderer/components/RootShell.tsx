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
  return (
    <>
      <HotkeyRegistrar />
      <ConfirmingActionOverlayBridge />
      <Outlet />
      <ToastContainer />
    </>
  );
}

export function RootShell(): React.ReactElement {
  return (
    <OverlayProvider>
      <RootShellInner />
    </OverlayProvider>
  );
}
