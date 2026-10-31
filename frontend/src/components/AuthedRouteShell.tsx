// Authed-layout component: chrome (AppFooter, overlays) + Outlet.
// Navigation is 100% router-owned — there is no mode or coherence machinery.
// OverlayProvider, HotkeyRegistrar, ConfirmingActionOverlayBridge, and
// ToastContainer live in RootShell (root route component).

import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router';
import React from 'react';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import { routeBaseFromPath } from '../utils/keybindings';
import { AppFooter } from './AppFooter';
import { CommandPalette } from './CommandPalette';
import { FirstRunRail } from './FirstRunRail';
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

  const routeBase = routeBaseFromPath(routerState.location.pathname);
  const isRailVisible =
    useStore(
      settingsStore,
      (s) => s.appearance.onboardingComplete && !s.appearance.tourComplete,
    ) &&
    (routeBase === 'list' || routeBase === 'detail' || routeBase === 'diff');

  return (
    <div className="flex h-full w-full flex-col bg-surface text-on-surface">
      <div className="flex-1 min-h-0 flex flex-col lg:flex-row">
        <div className="flex-1 min-h-0 min-w-0 flex flex-col">
          <Outlet />
        </div>
        {isRailVisible && <FirstRunRail />}
      </div>
      <AppFooter />
      <CommandPalette />
      <RepoPickerDialog />
      <HelpOverlay />
    </div>
  );
}
