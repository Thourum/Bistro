import {
  DevFolderScan,
  ReviewQueue as ReviewQueueService,
  Shell,
} from '@bindings/services';
import { type Fault, ok, type ResultAsync } from '@thourum/fault';
import type { CancellablePromise } from '@wailsio/runtime';
import type {
  ScanDevFolderResult,
  ValidateDevFolderResult,
} from '../pages/onboardingResults';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import type { OnboardingSyncState, UIState } from '../store/ui';
import type { RepoInfo, ReviewQueue } from '../types';
import { createLogger } from '../utils/logger';
import {
  buildOnboardingWatchChanges,
  type OnboardingSelection,
  sealOnboardingSelection,
} from '../utils/onboardingRepos';
import { call } from './engine/call';
import { loadRepos, setRepoWatched } from './repos';
import { addToast } from './toasts';

const REVIEW_QUEUE_TIMEOUT_MS = 35_000;
const log = createLogger('OnboardingSync');

export function runStartupDevFolderScan(): void {
  const path = settingsStore.state.repositories.devFolderPath;
  if (!path) return;
  void call(() => DevFolderScan.Scan({ path, reason: 'startup' }), {
    timeoutMs: null,
  }).match(
    () => undefined,
    (fault) =>
      log.warn({ err: fault.message }, 'startup dev-folder scan failed'),
  );
}

export type FolderScanState =
  | { readonly status: 'none' }
  | { readonly status: 'choosing' }
  | { readonly status: 'scanning'; readonly path: string }
  | {
      readonly status: 'done';
      readonly path: string;
      readonly partial: boolean;
    }
  | {
      readonly status: 'error';
      readonly path: string;
      readonly reason: string;
    };

export function requestCachedOnboardingRepos(): ResultAsync<
  readonly RepoInfo[],
  Fault
> {
  return loadRepos({ cacheOnly: true });
}

export function requestOnboardingReviewQueue(): ResultAsync<
  ReviewQueue,
  Fault
> {
  return call(() => ReviewQueueService.Get(), {
    timeoutMs: REVIEW_QUEUE_TIMEOUT_MS,
  });
}

export function chooseOnboardingFolder(): ResultAsync<
  ValidateDevFolderResult | null,
  Fault
> {
  return call(() => Shell.PickFolder())
    .andThen((path) =>
      path === '' ? ok(null) : call(() => DevFolderScan.Validate({ path })),
    )
    .andThen((result) =>
      result === null
        ? ok(null)
        : updateSettings({
            repositories: { devFolderPath: result.canonical_path },
          }).map(() => ({ canonicalPath: result.canonical_path })),
    );
}

export function scanOnboardingFolder(
  path: string,
  onStart: (promise: CancellablePromise<unknown>) => void,
): ResultAsync<ScanDevFolderResult, Fault> {
  return call(() => DevFolderScan.Scan({ path, reason: 'settings-change' }), {
    timeoutMs: null,
    onStart,
  });
}

export function stopOnboardingScan(promise: CancellablePromise<unknown>): void {
  void promise.cancel();
}

export function commitOnboardingSelection(
  selection: OnboardingSelection,
): void {
  for (const change of buildOnboardingWatchChanges(selection)) {
    const repo = selection.repos.find(
      (repository) => repository.id === change.repo_id,
    );
    if (repo === undefined) continue;
    void setRepoWatched(change.repo_id, change.watched).match(
      () => undefined,
      () => {
        addToast({
          type: 'error',
          dismissible: true,
          message: `Couldn't switch ${repo.owner}/${repo.repo} ${change.watched ? 'on' : 'off'}. Change it in Settings → Repos.`,
        });
      },
    );
  }
}

export function updateOnboardingSyncState(
  reduce: (state: OnboardingSyncState | null) => OnboardingSyncState | null,
): void {
  appStore.setState((state) => ({
    ...state,
    onboardingSync: reduce(state.onboardingSync),
  }));
}

export function updateOnboardingSelection(
  reduce: (state: OnboardingSelection) => OnboardingSelection,
): void {
  updateOnboardingSyncState((state) =>
    state === null ? null : { ...state, selection: reduce(state.selection) },
  );
}

export function advanceOnboardingToPractice(): void {
  const selection = appStore.state.onboardingSync?.selection;
  if (selection === undefined || selection.sealed) return;
  updateOnboardingSelection(sealOnboardingSelection);
  commitOnboardingSelection(selection);
  appStore.setState((state) => ({ ...state, onboardingStep: 'practice' }));
}

/** Gates the onboarding hotkeys; the page sets it on mount and clears it on unmount. */
export function setOnboardingStep(step: UIState['onboardingStep']): void {
  appStore.setState((state) => ({ ...state, onboardingStep: step }));
}

/** Clears a handled keyboard intent without consuming another intent. */
export function consumeOnboardingIntent(
  intent: UIState['onboardingIntent'],
): void {
  appStore.setState((state) =>
    state.onboardingIntent === intent
      ? { ...state, onboardingIntent: null }
      : state,
  );
}
