import { useHotkey } from '@tanstack/react-hotkeys';
import { err, ok, ResultAsync } from 'neverthrow';
import React from 'react';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import type { UIState } from '../store/ui';

/**
 * One-time onboarding — structure and classes match
 * `stitch_terminal_authentic_prd/onboarding/code.html` (body main + footer).
 * Global `AppFooter` renders below this component in `App.tsx`.
 */
interface ValidateDevFolderResult {
  canonicalPath: string;
}

interface ScanDevFolderResult {
  discovered: number;
}

function validationErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return 'Unable to validate that folder.';
}

function parseValidateDevFolderResult(data: unknown): ValidateDevFolderResult {
  if (
    typeof data === 'object' &&
    data !== null &&
    (data as { ok?: unknown }).ok === true &&
    typeof (data as { canonical_path?: unknown }).canonical_path === 'string'
  ) {
    return {
      canonicalPath: (data as { canonical_path: string }).canonical_path,
    };
  }
  throw new Error('Dev folder validation returned an invalid response.');
}

function parseScanDevFolderResult(data: unknown): ScanDevFolderResult {
  if (
    typeof data === 'object' &&
    data !== null &&
    (data as { ok?: unknown }).ok === true &&
    typeof (data as { discovered?: unknown }).discovered === 'number'
  ) {
    return {
      discovered: (data as { discovered: number }).discovered,
    };
  }
  throw new Error('Dev folder scan returned an invalid response.');
}

export function OnboardingPage(): React.ReactElement {
  const confirmingAction = useStore(
    appStore,
    (s: UIState) => s.confirmingAction,
  );
  const { request } = useWebSocket();
  const [devFolderError, setDevFolderError] = React.useState<string | null>(
    null,
  );
  const [discoveredCount, setDiscoveredCount] = React.useState<number | null>(
    null,
  );

  const finish = React.useCallback(() => {
    void updateSettings({
      appearance: {
        ...settingsStore.state.appearance,
        onboardingComplete: true,
      },
    });
  }, []);

  const chooseDevFolder = React.useCallback(() => {
    setDevFolderError(null);
    setDiscoveredCount(null);

    ResultAsync.fromPromise(
      window.electron.pickFolder(),
      validationErrorMessage,
    )
      .andThen((path) => {
        if (path === null) {
          return err('Folder selection cancelled.');
        }
        return ok(path);
      })
      .andThen((path) =>
        ResultAsync.fromPromise(
          request('VALIDATE_DEV_FOLDER', { path }).then(
            parseValidateDevFolderResult,
          ),
          validationErrorMessage,
        ),
      )
      .andThen((result) =>
        ResultAsync.fromPromise(
          updateSettings({
            repositories: { devFolderPath: result.canonicalPath },
          }).then(() => result),
          validationErrorMessage,
        ),
      )
      .andThen((result) =>
        ResultAsync.fromPromise(
          request('SCAN_DEV_FOLDER', {
            path: result.canonicalPath,
            reason: 'settings-change',
          }).then(parseScanDevFolderResult),
          validationErrorMessage,
        ),
      )
      .match(
        (result) => {
          setDiscoveredCount(result.discovered);
        },
        (message) => {
          if (message !== 'Folder selection cancelled.') {
            setDevFolderError(message);
          }
        },
      );
  }, [request]);

  React.useEffect(() => {
    if (confirmingAction === 'onboarding_finish') {
      appStore.setState((s) => ({ ...s, confirmingAction: null }));
      finish();
    }
  }, [confirmingAction, finish]);

  useHotkey('Space', finish);

  return (
    <div className="flex flex-col flex-1 min-h-0 h-full w-full bg-background text-on-surface">
      {/* Same as draft: <main class="flex-grow flex items-center justify-center p-6"> */}
      <main className="grow flex items-center justify-center p-6 min-h-0 w-full min-w-0 overflow-auto">
        <div className="max-w-3xl w-full">
          <header className="mb-16">
            <h1 className="text-5xl font-black tracking-tighter text-on-surface uppercase mb-2">
              BISTRO
            </h1>
            <p className="text-xl text-on-surface-variant font-medium max-w-lg leading-snug">
              Master the Technical Atelier. Your productivity is bound to your
              fluency with the terminal.
            </p>
          </header>

          <div className="grid grid-cols-1 md:grid-cols-12  bg-outline-variant/20 border border-outline-variant/30">
            <div className="md:col-span-7 bg-surface-container-lowest p-8 flex flex-col justify-between">
              <div>
                <div className="flex items-center gap-3 mb-8">
                  <span className="material-symbols-outlined text-primary">
                    keyboard
                  </span>
                  <span className="mono-text text-xs font-bold uppercase tracking-widest">
                    Core Movements
                  </span>
                </div>
                <div className="space-y-6">
                  <div className="flex items-center justify-between">
                    <span className="text-lg font-medium">Navigate Items</span>
                    <div className="flex gap-1">
                      <kbd className="mono-text px-3 py-1 bg-surface-container-low border-b-2 border-outline-variant text-sm font-bold">
                        j
                      </kbd>
                      <kbd className="mono-text px-3 py-1 bg-surface-container-low border-b-2 border-outline-variant text-sm font-bold">
                        k
                      </kbd>
                    </div>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-lg font-medium text-on-surface-variant">
                      Open Selected
                    </span>
                    <kbd className="mono-text px-3 py-1 bg-surface-container-low border-b-2 border-outline-variant text-sm font-bold">
                      Enter
                    </kbd>
                  </div>
                </div>
              </div>
              <div className="mt-12 pt-8 border-t border-outline-variant/10">
                <p className="mono-text text-[10px] text-outline leading-relaxed uppercase">
                  Linear progression through the stack using standard vim-like
                  bindings for peak ergonomic efficiency.
                </p>
              </div>
            </div>

            <div className="md:col-span-5 bg-surface-container-low p-8">
              <div className="flex items-center gap-3 mb-8">
                <span className="material-symbols-outlined text-primary">
                  terminal
                </span>
                <span className="mono-text text-xs font-bold uppercase tracking-widest">
                  Omni-Search
                </span>
              </div>
              <div className="flex flex-col items-center justify-center py-4 bg-surface-container-lowest border border-outline-variant/30 mb-6">
                <div className="flex gap-2">
                  <kbd className="mono-text px-3 py-1 bg-surface-container text-sm font-bold">
                    ⌘
                  </kbd>
                  <kbd className="mono-text px-3 py-1 bg-surface-container text-sm font-bold">
                    K
                  </kbd>
                </div>
              </div>
              <p className="text-sm font-medium mb-2">Open command box</p>
              <p className="mono-text text-[10px] text-outline uppercase tracking-tight leading-normal">
                Global entry point for files, settings, and repository commands.
              </p>
            </div>

            <div className="md:col-span-6 bg-surface-container-low p-8 border-t border-outline-variant/30">
              <div className="flex items-center justify-between">
                <div className="flex flex-col">
                  <span className="text-lg font-medium mb-1">Approve</span>
                  <span className="mono-text text-[10px] text-outline uppercase">
                    Rapid Review Cycle
                  </span>
                </div>
                <kbd className="mono-text px-4 py-2 bg-primary text-on-primary font-bold text-lg">
                  a
                </kbd>
              </div>
            </div>

            <div className="md:col-span-6 bg-surface-container-lowest p-8 border-t border-l border-outline-variant/30">
              <div className="flex items-center justify-between">
                <div className="flex flex-col">
                  <span className="text-lg font-medium mb-1 text-on-surface-variant">
                    Comment
                  </span>
                  <span className="mono-text text-[10px] text-outline uppercase">
                    Annotate Changes
                  </span>
                </div>
                <kbd className="mono-text px-4 py-2 border-2 border-outline text-outline font-bold text-lg">
                  c
                </kbd>
              </div>
            </div>
          </div>

          <div className="mt-12 bg-surface-container-lowest border border-outline-variant/30 p-8">
            <div className="flex items-center gap-3 mb-4">
              <span className="material-symbols-outlined text-primary">
                folder_open
              </span>
              <span className="mono-text text-xs font-bold uppercase tracking-widest">
                Use your existing projects folder
              </span>
            </div>
            <p className="text-sm text-on-surface-variant leading-relaxed mb-3">
              Bistro registers every GitHub repo found in this folder and starts
              syncing PRs for healthy discoveries.
            </p>
            <p className="mono-text text-[10px] text-outline uppercase tracking-tight leading-normal mb-6">
              Bistro fetches PR objects into these repos only under
              refs/bistro/* and never touches branches, tags, or files. Unwanted
              repos can be removed afterwards via the repo recovery/remove flow.
            </p>
            {devFolderError !== null ? (
              <p className="text-sm text-error font-medium mb-4" role="alert">
                {devFolderError}
              </p>
            ) : null}
            {discoveredCount !== null ? (
              <p className="text-sm text-primary font-medium mb-4">
                {discoveredCount}{' '}
                {discoveredCount === 1
                  ? 'repository discovered'
                  : 'repositories discovered'}
              </p>
            ) : null}
            <div className="flex flex-wrap gap-3">
              <button
                type="button"
                onClick={chooseDevFolder}
                className="mono-text px-4 py-2 bg-primary text-on-primary font-bold text-xs uppercase tracking-widest hover:opacity-90 transition-opacity"
              >
                Choose folder
              </button>
              <button
                type="button"
                onClick={finish}
                className="mono-text px-4 py-2 border border-outline-variant text-outline font-bold text-xs uppercase tracking-widest hover:bg-surface-container transition-colors"
              >
                Skip
              </button>
            </div>
          </div>

          <div className="mt-12 w-full h-1 bg-surface-container-highest">
            <div className="h-full bg-primary w-1/4" />
          </div>
        </div>
      </main>

      {/* Same as draft: <footer class="bg-surface-container-lowest py-8 ..."> */}
      <footer className="bg-surface-container-lowest py-8 flex flex-col items-center border-t border-outline-variant/20 shrink-0">
        <div className="flex flex-col items-center gap-4">
          <button
            type="button"
            onClick={finish}
            className="flex items-center gap-2 hover:opacity-80 transition-opacity"
          >
            <span className="mono-text text-[11px] font-bold text-outline uppercase tracking-widest">
              Press
            </span>
            <kbd className="mono-text px-8 py-2 bg-on-surface text-surface text-sm font-bold tracking-widest uppercase">
              Space
            </kbd>
            <span className="mono-text text-[11px] font-bold text-outline uppercase tracking-widest">
              to finish
            </span>
          </button>
          <div className="flex gap-8 mt-4">
            <span className="mono-text text-[9px] text-outline uppercase tracking-tighter opacity-50">
              v0.1.0-release
            </span>
            <span className="mono-text text-[9px] text-outline uppercase tracking-tighter opacity-50">
              AUTH: VERIFIED
            </span>
          </div>
        </div>
      </footer>
    </div>
  );
}
