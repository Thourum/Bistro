import { useCallback, useEffect, useRef, useState } from 'react';
import { useStore } from '../hooks/useStore';
import {
  advanceOnboardingToPractice,
  chooseOnboardingFolder,
  consumeOnboardingIntent,
  type FolderScanState,
  requestCachedOnboardingRepos,
  requestOnboardingReviewQueue,
  scanOnboardingFolder,
  stopOnboardingScan,
  updateOnboardingSyncState,
} from '../services/onboardingSync';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { OnboardingSyncState } from '../store/ui';
import type { RepoInfo } from '../types';
import {
  createOnboardingSelection,
  mergeOnboardingQueue,
  mergeOnboardingRepos,
} from '../utils/onboardingRepos';

interface OnboardingSyncControls {
  readonly chooseFolder: () => void;
  readonly stopScan: () => void;
  readonly scanAgain: () => void;
}
interface OnboardingSync extends OnboardingSyncControls {
  readonly state: OnboardingSyncState;
  readonly canContinue: boolean;
  readonly continueFromSync: () => void;
}
interface FolderLifecycleOptions {
  readonly isDisposed: () => boolean;
  readonly refreshRepos: () => void;
  readonly initialFolder: FolderScanState;
}

function localRepos(repos: readonly RepoInfo[]): readonly RepoInfo[] {
  return repos.filter((repo) => Boolean(repo.local_path?.trim()));
}

function initialSyncState(): OnboardingSyncState {
  const path = settingsStore.state.repositories.devFolderPath;
  return {
    selection: mergeOnboardingRepos(
      createOnboardingSelection(),
      localRepos(appStore.state.repos),
    ),
    initialReposPending: true,
    repoError: null,
    queuePending: true,
    queueError: null,
    folder: path
      ? { status: 'done', path, partial: false }
      : { status: 'none' },
  };
}

function startFolderLifecycle(
  options: FolderLifecycleOptions,
): OnboardingSyncControls {
  let scanAttempt = 0;
  let scanID: string | undefined;
  let choiceAttempt = 0;
  let previousFolder = options.initialFolder;

  function setFolder(folder: FolderScanState): void {
    if (appStore.state.onboardingSync?.folder.status === 'choosing') {
      previousFolder = folder;
      return;
    }
    updateOnboardingSyncState((state) =>
      state === null ? null : { ...state, folder },
    );
  }

  function scanFolder(path: string): void {
    const attempt = ++scanAttempt;
    scanID = undefined;
    updateOnboardingSyncState((state) =>
      state === null
        ? null
        : { ...state, folder: { status: 'scanning', path } },
    );
    // A new engine scan serializes old persistence without a separate Stop.
    void scanOnboardingFolder(path, (id) => {
      if (!options.isDisposed() && attempt === scanAttempt) scanID = id;
    }).match(
      (result) => {
        if (options.isDisposed() || attempt !== scanAttempt) return;
        setFolder({ status: 'done', path, partial: result.partial });
        options.refreshRepos();
      },
      (error) => {
        if (options.isDisposed() || attempt !== scanAttempt) return;
        setFolder({ status: 'error', path, reason: error.message });
        options.refreshRepos();
      },
    );
  }

  function chooseFolder(): void {
    const folder = appStore.state.onboardingSync?.folder;
    if (folder === undefined || folder.status === 'choosing') return;
    const attempt = ++choiceAttempt;
    previousFolder = folder;
    updateOnboardingSyncState((state) =>
      state === null ? null : { ...state, folder: { status: 'choosing' } },
    );
    void chooseOnboardingFolder().match(
      (result) => {
        if (options.isDisposed() || attempt !== choiceAttempt) return;
        if (result === null) {
          updateOnboardingSyncState((state) =>
            state === null ? null : { ...state, folder: previousFolder },
          );
          return;
        }
        scanFolder(result.canonicalPath);
      },
      (error) => {
        if (options.isDisposed() || attempt !== choiceAttempt) return;
        const path = 'path' in previousFolder ? previousFolder.path : '';
        updateOnboardingSyncState((state) =>
          state === null
            ? null
            : {
                ...state,
                folder: { status: 'error', path, reason: error.message },
              },
        );
      },
    );
  }

  return {
    chooseFolder,
    stopScan: () => {
      if (
        appStore.state.onboardingSync?.folder.status === 'scanning' &&
        scanID !== undefined
      )
        stopOnboardingScan(scanID);
    },
    scanAgain: () => {
      const folder = appStore.state.onboardingSync?.folder;
      if (
        folder !== undefined &&
        (folder.status === 'done' || folder.status === 'error') &&
        folder.path
      )
        scanFolder(folder.path);
    },
  };
}

function startOnboardingLifecycle(initial: OnboardingSyncState): {
  readonly controls: OnboardingSyncControls;
  readonly dispose: () => void;
} {
  updateOnboardingSyncState(() => initial);
  let disposed = false;
  let queueSettled = false;
  let cached: readonly RepoInfo[] = appStore.state.repos;
  let cacheAttempt = 0;

  function refreshRepos(): void {
    const attempt = ++cacheAttempt;
    void requestCachedOnboardingRepos().match(
      (repos) => {
        if (disposed || attempt !== cacheAttempt) return;
        cached = mergeOnboardingRepos(createOnboardingSelection(), [
          ...cached,
          ...repos,
        ]).repos;
        updateOnboardingSyncState((state) =>
          state === null
            ? null
            : {
                ...state,
                initialReposPending: false,
                repoError: null,
                ...(queueSettled ? { queuePending: false } : {}),
                selection: mergeOnboardingRepos(
                  state.selection,
                  queueSettled ? cached : localRepos(cached),
                ),
              },
        );
      },
      (error) => {
        if (disposed || attempt !== cacheAttempt) return;
        updateOnboardingSyncState((state) =>
          state === null
            ? null
            : {
                ...state,
                initialReposPending: false,
                repoError: error.message,
                ...(queueSettled ? { queuePending: false } : {}),
              },
        );
      },
    );
  }

  const controls = startFolderLifecycle({
    isDisposed: () => disposed,
    refreshRepos,
    initialFolder: initial.folder,
  });
  refreshRepos();
  void requestOnboardingReviewQueue().match(
    (queue) => {
      if (disposed) return;
      queueSettled = true;
      // Continue waits for this refresh so queue-discovered repos get defaults.
      updateOnboardingSyncState((state) =>
        state === null
          ? null
          : {
              ...state,
              queueError: null,
              selection: mergeOnboardingRepos(
                mergeOnboardingQueue(state.selection, queue),
                cached,
              ),
            },
      );
      refreshRepos();
    },
    (error) => {
      if (disposed) return;
      queueSettled = true;
      updateOnboardingSyncState((state) =>
        state === null
          ? null
          : {
              ...state,
              queuePending: false,
              queueError: error.message,
              selection: mergeOnboardingRepos(state.selection, cached),
            },
      );
    },
  );
  return {
    controls,
    dispose: () => {
      disposed = true;
      updateOnboardingSyncState(() => null);
      consumeOnboardingIntent(appStore.state.onboardingIntent);
    },
  };
}

/** Owns data lifetimes once for both onboarding steps; the engine owns scans. */
export function useOnboardingSync(): OnboardingSync {
  const [initial] = useState(initialSyncState);
  const stored = useStore(appStore, (state) => state.onboardingSync);
  const intent = useStore(appStore, (state) => state.onboardingIntent);
  const controls = useRef<OnboardingSyncControls | null>(null);
  useEffect(() => {
    const lifecycle = startOnboardingLifecycle(initial);
    controls.current = lifecycle.controls;
    return () => {
      controls.current = null;
      lifecycle.dispose();
    };
  }, [initial]);
  const chooseFolder = useCallback(() => controls.current?.chooseFolder(), []);
  const stopScan = useCallback(() => controls.current?.stopScan(), []);
  const scanAgain = useCallback(() => controls.current?.scanAgain(), []);
  useEffect(() => {
    if (intent !== 'chooseFolder' && intent !== 'stopScan') return;
    consumeOnboardingIntent(intent);
    if (intent === 'chooseFolder') chooseFolder();
    else stopScan();
  }, [intent, chooseFolder, stopScan]);

  const state = stored ?? initial;
  const canContinue =
    !state.queuePending &&
    !state.selection.sealed &&
    !(state.initialReposPending && state.selection.repos.length === 0);
  const continueFromSync = useCallback(() => {
    if (canContinue) advanceOnboardingToPractice();
  }, [canContinue]);
  return {
    state,
    canContinue,
    chooseFolder,
    stopScan,
    scanAgain,
    continueFromSync,
  };
}
