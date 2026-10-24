import { useMatchRoute, useNavigate } from '@tanstack/react-router';
import { Command } from 'cmdk';
import type React from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { useVisiblePRs } from '../hooks/useVisiblePRs';
import { useOverlays } from '../overlays/OverlayContext';
import { wsClient } from '../services/ws/connection';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import type { UIState } from '../store/ui';
import { initialUIState } from '../store/ui';
import type { PR } from '../types';
import { focusedExpertisePR, repoIdForPR } from '../utils/expertise';
import { findActionKeys } from '../utils/legendPresets';
import { createLogger } from '../utils/logger';
import { openUrlInBrowser } from '../utils/openUrl';
import { prBrowserUrl } from '../utils/triage';
import { addToast } from './ToastContainer';

const log = createLogger('CommandPalette');

type Icon = string;

interface CommandItem {
  readonly id: string;
  readonly name: string;
  readonly description?: string;
  readonly icon: Icon;
  /** Action whose effective keys render as the row's shortcut hint. */
  readonly shortcutActionId?: string;
  readonly dev?: boolean;
  /** Only listed on the diff route — DiffPage is the sole consumer of the intent. */
  readonly diffOnly?: boolean;
}

const isDev = import.meta.env.DEV;

const COMMANDS: readonly CommandItem[] = [
  {
    id: 'help',
    name: 'Show keyboard shortcuts',
    icon: 'keyboard',
    shortcutActionId: 'global.help',
  },
  {
    id: 'tour',
    name: ':tour',
    description: 'Replay the first-run guided tour',
    icon: 'school',
  },
  {
    id: 'settings',
    name: 'Open project settings',
    icon: 'settings',
    shortcutActionId: 'global.settings',
  },
  {
    id: 'sync',
    name: 'Sync now',
    description: 'Force sync all watched repos',
    icon: 'sync',
  },
  {
    id: 'clear',
    name: 'Clear all filters and search',
    icon: 'filter_alt_off',
  },
  {
    id: 'view-lanes',
    name: ':view lanes',
    description: 'Organize the PR inbox by review lanes',
    icon: 'view_agenda',
  },
  {
    id: 'view-groups',
    name: ':view groups',
    description: 'Organize the PR inbox by repository',
    icon: 'account_tree',
  },
  {
    id: 'open-browser',
    name: ':open in browser',
    description: 'Open the focused PR on GitHub',
    icon: 'open_in_new',
    shortcutActionId: 'list.openBrowser',
  },
  {
    id: 'diff-split',
    name: ':diff split',
    description: 'Show diffs side by side',
    icon: 'vertical_split',
    shortcutActionId: 'diff.toggleStyle',
  },
  {
    id: 'diff-unified',
    name: ':diff unified',
    description: 'Show diffs in a single column',
    icon: 'view_stream',
    shortcutActionId: 'diff.toggleStyle',
  },
  {
    id: 'open-editor',
    name: 'Open file in editor',
    description:
      "Opens in $EDITOR at the selected comment's line when one is selected",
    icon: 'edit_document',
    diffOnly: true,
  },
  {
    id: 'repo-picker',
    name: 'Open repository picker',
    description: 'Discover and watch repositories',
    icon: 'source_environment',
  },
  {
    id: 'add-repo-by-url',
    name: 'Add repository by URL',
    description: 'Clone or link a repository from a Git URL',
    icon: 'add_link',
  },
  { id: 'expertise-enable', name: 'Enable expertise', icon: 'psychology' },
  { id: 'expertise-disable', name: 'Disable expertise', icon: 'psychology' },
  { id: 'focus-list', name: 'Focus PR list', icon: 'list_alt' },
  { id: 'focus-detail', name: 'Focus PR detail', icon: 'article' },
  { id: 'about', name: 'About BISTRO', icon: 'info' },
  {
    id: 'reset-store',
    name: 'Reset store to initial state',
    icon: 'restart_alt',
    description: 'DEV ONLY',
    dev: true,
  },
];

/**
 * Command Palette — matches `command_palette/code.html`.
 *
 * Built on the `cmdk` library: the root `<Command>` owns keyboard navigation
 * (arrow keys / Enter), the selection highlight, and accessibility. We pass
 * `shouldFilter={false}` and keep our own substring filtering (`filteredPRs` /
 * `filteredCommands`), so cmdk renders only the already-filtered items.
 *
 * Overlay: `fixed inset-0` with a transparent, blurred backdrop so the app
 * stays visible behind it; the box sits ~153px from the top in a `max-w-2xl`
 * container. Results are grouped into _Recently opened PRs_ and
 * _Available commands_; Arrow keys move between groups; Enter selects;
 * Escape closes.
 */
export function CommandPalette(): React.ReactElement | null {
  const {
    activeOverlay,
    closeOverlay,
    openHelp,
    openRepoPicker,
    openAddRepo,
    openTour,
  } = useOverlays();
  const isOpen = activeOverlay === 'command';
  const [query, setQuery] = useState('');
  const prefetchedRef = useRef(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const matchRoute = useMatchRoute();
  const routedPR = matchRoute({ to: '/prs/$prId', fuzzy: true });
  const isDiffRoute = matchRoute({ to: '/prs/$prId/diff' }) !== false;

  // Use the shared visible-PR source so palette results match the list route.
  const { prs } = useVisiblePRs();
  const selectedPRId = useStore(
    appStore,
    (state: UIState) => state.visiblePRIds[state.selectedPRIndex] || undefined,
  );
  const prList = useStore(settingsStore, (state) => state.prList);
  const bindings = useEffectiveBindings();
  const recentPRs = useMemo(() => prs.slice(0, 5), [prs]);

  const filteredPRs = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q === '') return recentPRs;
    return recentPRs.filter(
      (p) =>
        p.title.toLowerCase().includes(q) ||
        p.author.toLowerCase().includes(q) ||
        String(p.number).includes(q),
    );
  }, [recentPRs, query]);

  const filteredCommands = useMemo(() => {
    const q = query.trim().toLowerCase();
    return COMMANDS.filter((cmd) => {
      if (cmd.dev === true && !isDev) return false;
      if (cmd.diffOnly === true && !isDiffRoute) return false;
      if (q === '') return true;
      return (
        cmd.name.toLowerCase().includes(q) ||
        cmd.description?.toLowerCase().includes(q) === true
      );
    });
  }, [query, isDiffRoute]);

  useEffect(() => {
    if (isOpen) {
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [isOpen]);

  useEffect(() => {
    if (query.toLowerCase().startsWith('repo') && !prefetchedRef.current) {
      prefetchedRef.current = true;
      appStore.setState((s) => ({ ...s, reposLoading: true }));
      const requestId = wsClient.send('REQUEST_REPO_LIST', {});
      if (requestId === undefined) {
        appStore.setState((s) => ({ ...s, reposLoading: false }));
        prefetchedRef.current = false;
      }
    }
  }, [query]);

  const handleClose = (): void => {
    closeOverlay();
    setQuery('');
    prefetchedRef.current = false;
  };

  const handleExecutePR = (pr: PR): void => {
    // Navigate by stable pr.id — never write selectedPRIndex as identity.
    void navigate({ to: '/prs/$prId', params: { prId: pr.id } });
    closeOverlay();
    setQuery('');
  };

  const focusedPR = useMemo(
    () =>
      focusedExpertisePR(
        prs,
        routedPR === false ? undefined : routedPR.prId,
        selectedPRId,
      ),
    [routedPR, prs, selectedPRId],
  );

  const sendExpertiseCommand = (
    type: 'EXPERTISE_ENABLE' | 'EXPERTISE_DISABLE',
  ): void => {
    if (focusedPR === undefined) {
      openRepoPicker();
      setQuery('');
      return;
    }

    wsClient.send(type, { repo_id: repoIdForPR(focusedPR) });
    closeOverlay();
    setQuery('');
  };

  const updateSettingsFromCommand = (
    patch: Parameters<typeof updateSettings>[0],
  ): void => {
    updateSettings(patch).match(handleClose, (error) => {
      addToast({ type: 'error', message: error.message, dismissible: true });
    });
  };

  const handleExecuteCommand = (command: CommandItem): void => {
    switch (command.id) {
      case 'settings':
        void navigate({ to: '/settings' });
        closeOverlay();
        setQuery('');
        return;
      case 'help':
        openHelp('list');
        setQuery('');
        return;
      case 'tour':
        openTour();
        setQuery('');
        return;
      case 'focus-list':
        // Navigate to /prs; route↔mode coherence effect will correct mode.
        void navigate({
          to: '/prs',
          search: (prev: Record<string, unknown>) => prev,
        });
        closeOverlay();
        setQuery('');
        return;
      case 'focus-detail':
        // Cannot navigate to detail without a prId — this command is a no-op
        // when there is no current routed PR.
        setQuery('');
        return;
      case 'clear':
        appStore.setState((s) => ({
          ...s,
          searchQuery: '',
          filterState: 'open',
        }));
        closeOverlay();
        setQuery('');
        return;
      case 'view-lanes':
        updateSettingsFromCommand({ prList: { ...prList, view: 'flat' } });
        return;
      case 'view-groups':
        updateSettingsFromCommand({ prList: { ...prList, view: 'grouped' } });
        return;
      case 'open-browser': {
        const url = focusedPR === undefined ? null : prBrowserUrl(focusedPR);
        if (url !== null)
          void openUrlInBrowser(url).orInspect((error) =>
            log.error({ error: error.message }, 'failed to open PR in browser'),
          );
        closeOverlay();
        setQuery('');
        return;
      }
      case 'diff-split':
        updateSettingsFromCommand({ diff: { style: 'split' } });
        return;
      case 'diff-unified':
        updateSettingsFromCommand({ diff: { style: 'unified' } });
        return;
      case 'open-editor':
        // DiffPage owns the current file/line and the checkout dialog; the
        // palette only signals intent, same as diff.markViewed's 'v'.
        appStore.setState((s) => ({
          ...s,
          confirmingAction: 'open-in-editor',
        }));
        closeOverlay();
        setQuery('');
        return;
      case 'repo-picker':
        openRepoPicker();
        setQuery('');
        return;
      case 'add-repo-by-url':
        openAddRepo();
        setQuery('');
        return;
      case 'expertise-enable':
        sendExpertiseCommand('EXPERTISE_ENABLE');
        return;
      case 'expertise-disable':
        sendExpertiseCommand('EXPERTISE_DISABLE');
        return;
      case 'sync':
        wsClient.send('FORCE_SYNC', {});
        appStore.setState((s) => ({
          ...s,
          syncStatus: 'syncing',
        }));
        closeOverlay();
        setQuery('');
        return;
      case 'reset-store':
        appStore.setState(() => ({ ...initialUIState }));
        closeOverlay();
        setQuery('');
        return;
      default:
        handleClose();
    }
  };

  if (!isOpen) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Command palette"
      className="fixed inset-0 z-40 bg-transparent backdrop-blur-[20px] flex items-start justify-center pt-[153px]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) handleClose();
      }}
    >
      <Command
        shouldFilter={false}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.preventDefault();
            handleClose();
          }
        }}
        className="w-full max-w-2xl mx-4 bg-surface-container-lowest border border-primary shadow-ambient flex flex-col overflow-hidden"
      >
        {/* Search input header */}
        <div className="flex items-center gap-3 px-4 h-14 border-b border-outline-variant/20">
          <span className="material-symbols-outlined text-primary shrink-0">
            search
          </span>
          <Command.Input
            ref={inputRef}
            value={query}
            onValueChange={setQuery}
            placeholder="Type a command or search…"
            className="flex-1 min-w-0 bg-transparent appearance-none border-0 shadow-none outline-none ring-0 text-on-surface placeholder:text-outline text-lg focus:border-transparent focus:outline-none focus:ring-0 focus-visible:outline-none focus-visible:ring-0"
          />
          <div className="flex items-center gap-2 shrink-0 ml-3">
            <kbd className="font-mono text-[10px] px-1.5 py-0.5 bg-surface-container border border-outline-variant text-on-surface-variant">
              ESC
            </kbd>
            <span className="font-mono text-[10px] text-outline uppercase tracking-widest">
              to close
            </span>
          </div>
        </div>

        {/* Results */}
        <Command.List className="max-h-[614px] overflow-y-auto py-2">
          <Command.Empty className="px-6 py-6 text-center text-sm text-outline font-mono tracking-wide">
            No matches
          </Command.Empty>

          {filteredPRs.length > 0 && (
            <Command.Group
              heading="Recently opened PRs"
              className="px-2 [&_[cmdk-group-heading]]:font-mono [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:text-outline [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-widest [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-2 [&_[cmdk-group-heading]]:mb-2 [&_[cmdk-group-items]]:space-y-1"
            >
              {filteredPRs.map((pr) => (
                <PRResultRow
                  key={pr.id}
                  pr={pr}
                  onSelect={() => handleExecutePR(pr)}
                />
              ))}
            </Command.Group>
          )}

          {filteredCommands.length > 0 && (
            <Command.Group
              heading="Available commands"
              className="px-2 mt-4 [&_[cmdk-group-heading]]:font-mono [&_[cmdk-group-heading]]:text-[10px] [&_[cmdk-group-heading]]:text-outline [&_[cmdk-group-heading]]:uppercase [&_[cmdk-group-heading]]:tracking-widest [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-2 [&_[cmdk-group-heading]]:mb-2 [&_[cmdk-group-items]]:space-y-1"
            >
              {filteredCommands.map((cmd) => (
                <CommandRow
                  key={cmd.id}
                  command={cmd}
                  shortcut={
                    cmd.shortcutActionId === undefined
                      ? undefined
                      : findActionKeys(bindings, [cmd.shortcutActionId])
                  }
                  onSelect={() => handleExecuteCommand(cmd)}
                />
              ))}
            </Command.Group>
          )}
        </Command.List>

        {/* Footer */}
        <div className="px-4 h-8 bg-surface-container-low border-t border-outline-variant/20 flex items-center justify-between">
          <div className="flex gap-4">
            <div className="flex items-center gap-1">
              <span className="material-symbols-outlined text-[14px] text-outline">
                keyboard_arrow_up
              </span>
              <span className="material-symbols-outlined text-[14px] text-outline">
                keyboard_arrow_down
              </span>
              <span className="font-mono text-[9px] text-outline uppercase tracking-tight">
                Navigate
              </span>
            </div>
            <div className="flex items-center gap-1">
              <kbd className="font-mono text-[9px] px-1 bg-surface border border-outline-variant text-outline">
                ↩
              </kbd>
              <span className="font-mono text-[9px] text-outline uppercase tracking-tight">
                Select
              </span>
            </div>
          </div>
          <div className="font-mono text-[9px] text-outline uppercase tracking-widest">
            {`BISTRO // V${import.meta.env.VITE_APP_VERSION}`}
          </div>
        </div>
      </Command>
    </div>
  );
}

function PRResultRow({ pr, onSelect }: { pr: PR; onSelect: () => void }) {
  const relAgo = relativeTime(pr.updated_at ?? pr.created_at);
  return (
    <Command.Item
      value={`pr-${pr.id}`}
      onSelect={onSelect}
      className="group w-full flex items-center justify-between px-2 py-3 text-left cursor-pointer transition-colors border-l-2 border-transparent hover:bg-surface-container-highest data-[selected=true]:bg-surface-container-highest data-[selected=true]:border-primary"
    >
      <div className="flex items-center gap-3 min-w-0">
        <span className="material-symbols-outlined text-primary shrink-0">
          rebase
        </span>
        <div className="flex flex-col min-w-0">
          <span className="text-sm font-medium text-on-surface truncate">
            {pr.title}
          </span>
          <span className="font-mono text-[10px] text-outline truncate">
            {pr.owner}/{pr.repo}#{pr.number} • {relAgo} by @{pr.author}
          </span>
        </div>
      </div>
      <div className="flex items-center gap-2 shrink-0 ml-4 opacity-0 group-hover:opacity-100 group-data-[selected=true]:opacity-100 transition-opacity">
        <span className="font-mono text-[10px] text-outline uppercase">
          Select
        </span>
        <kbd className="font-mono text-[10px] px-1.5 py-0.5 bg-surface border border-outline-variant text-on-surface-variant">
          ENTER
        </kbd>
      </div>
    </Command.Item>
  );
}

function CommandRow({
  command,
  shortcut,
  onSelect,
}: {
  command: CommandItem;
  shortcut: string | undefined;
  onSelect: () => void;
}) {
  return (
    <Command.Item
      value={`cmd-${command.id}`}
      onSelect={onSelect}
      className="group w-full flex items-center justify-between px-2 py-3 text-left cursor-pointer transition-colors border-l-2 border-transparent hover:bg-surface-container-highest data-[selected=true]:bg-surface-container-highest data-[selected=true]:border-primary"
    >
      <div className="flex items-center gap-3 min-w-0">
        <span className="material-symbols-outlined text-on-surface-variant shrink-0">
          {command.icon}
        </span>
        <div className="flex flex-col min-w-0">
          <span className="text-sm font-medium text-on-surface truncate">
            {command.name}
          </span>
          {command.description !== undefined && (
            <span className="font-mono text-[10px] text-outline truncate">
              {command.description}
            </span>
          )}
        </div>
      </div>
      {shortcut !== undefined && (
        <kbd className="font-mono text-[10px] px-1.5 py-0.5 shrink-0 ml-4 opacity-40 group-hover:opacity-100 group-data-[selected=true]:opacity-100 transition-opacity bg-surface border border-outline-variant text-on-surface-variant">
          {shortcut}
        </kbd>
      )}
    </Command.Item>
  );
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const sec = Math.floor(diff / 1000);
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour}h ago`;
  const day = Math.floor(hour / 24);
  return `${day}d ago`;
}
