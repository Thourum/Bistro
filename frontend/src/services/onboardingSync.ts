import {
  DevFolderScan,
  ReviewQueue as ReviewQueueService,
} from '@bindings/services';
import { Fault, ok, ResultAsync } from '@thourum/fault';
import { z } from 'zod';
import { addToast } from '../components/ToastContainer';
import {
  parseScanDevFolderResult,
  parseValidateDevFolderResult,
  type ScanDevFolderResult,
  type ValidateDevFolderResult,
} from '../pages/onboardingResults';
import { appStore } from '../store';
import { settingsStore, updateSettings } from '../store/settings';
import type { OnboardingSyncState, UIState } from '../store/ui';
import type { RepoInfo, ReviewQueue } from '../types';
import { parseResponse } from '../utils/error';
import { createLogger } from '../utils/logger';
import {
  buildOnboardingWatchChanges,
  type OnboardingSelection,
  sealOnboardingSelection,
} from '../utils/onboardingRepos';
import { call } from './engine/call';
import { loadRepos, setRepoWatched } from './repos';
import { wsClient } from './ws/connection';

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
  return ResultAsync.fromPromise(window.electron.pickFolder(), (cause) =>
    new Fault('Unable to choose a folder.')
      .withTag('EXTERNAL_ERROR')
      .withCause(cause),
  )
    .andThen((path) => parseResponse(z.string().nullable(), path))
    .andThen((path) =>
      path === null
        ? ok(null)
        : wsClient
            .request('VALIDATE_DEV_FOLDER', { path })
            .andThen(parseValidateDevFolderResult)
            .andThen((result) =>
              updateSettings({
                repositories: {
                  ...settingsStore.state.repositories,
                  devFolderPath: result.canonicalPath,
                },
              }).map(() => result),
            ),
    );
}

export function scanOnboardingFolder(
  path: string,
  onSent: (scanID: string) => void,
): ResultAsync<ScanDevFolderResult, Fault> {
  return wsClient
    .request('SCAN_DEV_FOLDER', { path, reason: 'settings-change' }, { onSent })
    .andThen(parseScanDevFolderResult);
}

export function stopOnboardingScan(scanID: string): void {
  wsClient.send('SCAN_CANCEL', { scan_id: scanID });
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
