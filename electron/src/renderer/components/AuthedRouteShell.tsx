// Authed-layout component: chrome (AppFooter, overlays) + Outlet.
// Navigation is 100% router-owned — there is no mode or coherence machinery.
// OverlayProvider, HotkeyRegistrar, ConfirmingActionOverlayBridge, and
// ToastContainer live in RootShell (root route component).

import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router';
import React from 'react';
import { appStore } from '../store';
import { AppFooter } from './AppFooter';
import { CommandPalette } from './CommandPalette';
import { HelpOverlay } from './HelpOverlay';
import { RepoPickerDialog } from './RepoPickerDialog';

export function AuthedRouteShell(): React.ReactElement {
  const navigate = useNavigate();
  const routerState = useRouterState();

  // Pin to /prs on mount for cold-start (seeds search params from store).
  // Skipped for deep-links into /prs/$prId, /prs/$prId/diff, and /settings.
  // biome-ignore lint/correctness/useExhaustiveDependencies: intentional mount-only; navigate/routerState are stable
  React.useEffect(() => {
    const currentPath = routerState.location.pathname;
    if (/^\/prs\/[^/]+(\/diff)?$/.test(currentPath)) return;
    if (currentPath === '/settings') return;

    const { filterState, sortBy, searchQuery } = appStore.state;
    void navigate({
      to: '/prs',
      replace: true,
      search: { filter: filterState, sort: sortBy, q: searchQuery },
    });
  }, []);

  return (
    <div className="flex h-full w-full flex-col bg-surface text-on-surface">
      <div className="flex-1 min-h-0 flex flex-col">
        <Outlet />
      </div>
      <AppFooter />
      <CommandPalette />
      <RepoPickerDialog />
      <HelpOverlay />
    </div>
  );
}
