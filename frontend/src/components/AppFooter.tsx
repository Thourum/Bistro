// AppFooter is only rendered under AuthedRouteShell, which is mounted
// under the authed route guard — it is NEVER rendered on /login or /onboarding.
// Therefore it is always inside a RouterProvider and can call router hooks
// directly without an error-boundary wrapper.

import { useNavigate, useRouterState } from '@tanstack/react-router';
import { LoaderCircle } from 'lucide-react';
import React from 'react';
import { useStore } from '../hooks/useStore';
import { useOptionalOverlays } from '../overlays/OverlayContext';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { UIState } from '../store/ui';
import type { RouteBase } from '../utils/keybindings';
import { getEffectiveBindings, routeBaseFromPath } from '../utils/keybindings';
import { buildLegendForRouteBase } from '../utils/legendPresets';
import { useFooterSlotContent } from './FooterSlot';
import { SyncStatusHoverCard } from './SyncStatusHoverCard';

/**
 * Global bottom system bar for the authed layout.
 *
 * Three regions:
 *   ┌──────────────────────────┬──────────────────────┬────────────────────────────────┐
 *   │ LEFT: route-aware legend │ CENTER: page slot    │ RIGHT: status + shortcut chrome │
 *   └──────────────────────────┴──────────────────────┴────────────────────────────────┘
 */
export function AppFooter(): React.ReactElement {
  const syncStatus = useStore(appStore, (state: UIState) => state.syncStatus);
  const lastSyncAt = useStore(appStore, (state: UIState) => state.lastSyncAt);
  const isOffline = useStore(appStore, (state: UIState) => state.isOffline);
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const style = useStore(settingsStore, (s) => s.navigation.style);
  const syncSettings = useStore(settingsStore, (s) => s.sync);

  const overlayContext = useOptionalOverlays();
  const slots = useFooterSlotContent();
  const navigate = useNavigate();
  const routerState = useRouterState();

  const routeBase: RouteBase | null = routeBaseFromPath(
    routerState.location.pathname,
  );
  const activeOverlay = overlayContext?.activeOverlay ?? null;

  const legendEntries = React.useMemo(() => {
    if (slots.left) return [];
    const bindings = getEffectiveBindings(style, overrides);
    return buildLegendForRouteBase(routeBase, activeOverlay, bindings);
  }, [routeBase, activeOverlay, style, overrides, slots.left]);

  const { label: statusLabel, dotClass: statusDot } = deriveStatus(
    syncStatus,
    isOffline,
  );
  const isSyncing = syncStatus === 'syncing' && !isOffline;

  const handleShortcuts = () => {
    if (overlayContext) {
      const source =
        routeBase === 'detail' || routeBase === 'diff' ? routeBase : 'list';
      overlayContext.openHelp(source);
    }
  };

  return (
    <footer className="shrink-0 h-8 border-t border-outline-variant/40 bg-surface-container-high/80 backdrop-blur flex items-center z-50 text-[10px] uppercase tracking-widest font-mono">
      {/* LEFT — route-aware keybindings (or page-provided override) */}
      <div className="flex-1 min-w-0 flex items-center gap-4 px-4 overflow-hidden">
        {slots.left ?? <LegendStrip entries={legendEntries} />}
      </div>

      {/* CENTER — page slot (empty by default) */}
      <div className="flex-1 min-w-0 flex items-center justify-center px-4 overflow-hidden">
        {slots.center}
      </div>

      {/* RIGHT — global chrome: shortcuts / settings / status */}
      <div className="flex-1 min-w-0 flex items-center justify-end gap-4 px-4">
        <button
          type="button"
          onClick={handleShortcuts}
          className="text-outline hover:text-on-surface transition-colors"
        >
          Shortcuts
        </button>
        <button
          type="button"
          onClick={() => void navigate({ to: '/settings' })}
          className="text-outline hover:text-on-surface transition-colors"
        >
          Settings
        </button>
        <SyncStatusHoverCard
          statusLabel={statusLabel}
          syncStatus={syncStatus}
          isOffline={isOffline}
          lastSyncAt={lastSyncAt}
          autoSync={syncSettings.autoSync}
          syncIntervalSeconds={syncSettings.syncInterval}
        >
          <button
            type="button"
            className="flex cursor-default appearance-none items-center gap-2 border-y-0 border-r-0 border-l border-outline-variant/40 bg-transparent p-0 pl-4 outline-none focus-visible:text-on-surface"
          >
            {isSyncing ? (
              <LoaderCircle
                className="h-3 w-3 text-yellow-500 animate-spin"
                aria-hidden="true"
              />
            ) : (
              <span className={`w-2 h-2 ${statusDot}`} aria-hidden="true" />
            )}
          </button>
        </SyncStatusHoverCard>
      </div>
    </footer>
  );
}

interface LegendStripProps {
  entries: ReturnType<typeof buildLegendForRouteBase>;
}

function LegendStrip({ entries }: LegendStripProps): React.ReactElement | null {
  if (entries.length === 0) return null;
  return (
    <div className="flex items-center gap-4 overflow-hidden">
      {entries.map((entry) => (
        <span
          key={entry.actionId}
          className="flex items-center gap-2 whitespace-nowrap"
        >
          <kbd className="bg-surface-container-lowest px-1.5 py-0.5 text-[9px] border border-outline-variant text-on-surface font-mono">
            {entry.label}
          </kbd>
          <span className="text-outline">{entry.description}</span>
        </span>
      ))}
    </div>
  );
}

function deriveStatus(
  syncStatus: UIState['syncStatus'],
  isOffline: boolean,
): { label: string; dotClass: string } {
  if (isOffline) return { label: 'OFFLINE', dotClass: 'bg-outline' };
  switch (syncStatus) {
    case 'ready':
    case 'connected':
      return { label: 'OPERATIONAL', dotClass: 'bg-green-500' };
    case 'connecting':
      return { label: 'CONNECTING', dotClass: 'bg-yellow-500 animate-pulse' };
    case 'syncing':
      return { label: 'SYNCING', dotClass: 'bg-yellow-500 animate-pulse' };
    case 'rate_limited':
      return { label: 'RATE-LIMITED', dotClass: 'bg-error' };
    case 'error':
      return { label: 'ERROR', dotClass: 'bg-error' };
    default:
      return { label: 'IDLE', dotClass: 'bg-outline-variant' };
  }
}
