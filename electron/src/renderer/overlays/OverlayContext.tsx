// OverlayContext
//
// Overlays are local UI state (not routes or navigation modes).
// Single activeOverlay at a time.
// Components consume useOverlays(); hotkeys gate on route+overlay via isActive.

import type React from 'react';
import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
} from 'react';
import { appStore } from '../store';

export type OverlayKind =
  | 'command'
  | 'help'
  | 'search'
  | 'filter'
  | 'repoPicker';
export type HelpSourceRoute = 'list' | 'detail';

export interface OverlayContextValue {
  activeOverlay: OverlayKind | null;
  helpSourceRoute: HelpSourceRoute;
  openCommand(): void;
  openHelp(source: HelpSourceRoute): void;
  openSearch(): void;
  openFilter(): void;
  toggleFilter(): void;
  openRepoPicker(): void;
  closeOverlay(): void;
  closeSearch(opts?: { clearQuery?: boolean }): void;
}

const OverlayContext = createContext<OverlayContextValue | null>(null);

export function OverlayProvider({
  children,
}: {
  children: React.ReactNode;
}): React.ReactElement {
  const [activeOverlay, setActiveOverlay] = useState<OverlayKind | null>(null);
  const [helpSourceRoute, setHelpSourceRoute] =
    useState<HelpSourceRoute>('list');

  // Memoize stable action callbacks to avoid churning dependents
  // (ConfirmingActionOverlayBridge effect, useAppHotkeys refs, etc.).
  const openCommand = useCallback(() => setActiveOverlay('command'), []);
  const openHelp = useCallback((source: HelpSourceRoute) => {
    setHelpSourceRoute(source);
    setActiveOverlay('help');
  }, []);
  const openSearch = useCallback(() => setActiveOverlay('search'), []);
  const openFilter = useCallback(() => setActiveOverlay('filter'), []);
  const toggleFilter = useCallback(
    () => setActiveOverlay((prev) => (prev === 'filter' ? null : 'filter')),
    [],
  );
  const openRepoPicker = useCallback(() => setActiveOverlay('repoPicker'), []);
  const closeOverlay = useCallback(() => setActiveOverlay(null), []);
  const closeSearch = useCallback((opts?: { clearQuery?: boolean }) => {
    if (opts?.clearQuery) {
      appStore.setState((s) => ({ ...s, searchQuery: '' }));
    }
    setActiveOverlay(null);
  }, []);

  // Memoize the value object so identity is stable when state is unchanged.
  const value = useMemo<OverlayContextValue>(
    () => ({
      activeOverlay,
      helpSourceRoute,
      openCommand,
      openHelp,
      openSearch,
      openFilter,
      toggleFilter,
      openRepoPicker,
      closeOverlay,
      closeSearch,
    }),
    [
      activeOverlay,
      helpSourceRoute,
      openCommand,
      openHelp,
      openSearch,
      openFilter,
      toggleFilter,
      openRepoPicker,
      closeOverlay,
      closeSearch,
    ],
  );

  return (
    <OverlayContext.Provider value={value}>{children}</OverlayContext.Provider>
  );
}

export function useOverlays(): OverlayContextValue {
  const ctx = useContext(OverlayContext);
  if (!ctx) throw new Error('useOverlays must be used within OverlayProvider');
  return ctx;
}

/** Returns the overlay context or null if outside OverlayProvider. Safe to call anywhere. */
export function useOptionalOverlays(): OverlayContextValue | null {
  return useContext(OverlayContext);
}
