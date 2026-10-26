import type { ReactElement } from 'react';
import { ActionKeyHint } from '../components/ui/action-key-hint';
import { Button } from '../components/ui/button';
import type { FolderScanState } from '../services/onboardingSync';
import type { RepoInfo } from '../types';
import { countOnboardingFolderMatches } from '../utils/onboardingRepos';
import {
  OnboardingRepoTable,
  withoutTrailingPeriod,
} from './OnboardingRepoTable';
import type { useOnboardingSync } from './useOnboardingSync';

type OnboardingSync = ReturnType<typeof useOnboardingSync>;

const LABEL = 'mono-text text-[10px] uppercase tracking-widest';

function FolderPath({ path }: { readonly path: string }): ReactElement {
  // Canonical path unchanged: the renderer has no verified home directory for `~`.
  return (
    <span className="mono-text text-xs text-on-surface truncate" title={path}>
      {path}
    </span>
  );
}

function ChooseFolderButton({
  label,
  onClick,
}: {
  readonly label: string;
  readonly onClick: () => void;
}): ReactElement {
  return (
    <Button size="sm" variant="outline" onClick={onClick}>
      <span aria-hidden="true" className="material-symbols-outlined">
        folder_open
      </span>
      {label}
      <ActionKeyHint actionId="onboarding.chooseFolder" />
    </Button>
  );
}

function FolderControls({
  folder,
  repos,
  sync,
}: {
  readonly folder: FolderScanState;
  readonly repos: readonly RepoInfo[];
  readonly sync: OnboardingSync;
}): ReactElement {
  switch (folder.status) {
    case 'none':
      return (
        <ChooseFolderButton label="Choose folder" onClick={sync.chooseFolder} />
      );
    case 'choosing':
      return (
        <Button size="sm" variant="outline" disabled>
          Choosing folder…
        </Button>
      );
    case 'scanning':
      return (
        <div className="flex flex-col gap-3">
          <span className="flex items-center gap-2 min-w-0 text-xs text-on-surface">
            Scanning <FolderPath path={folder.path} />
          </span>
          <div
            role="progressbar"
            aria-label={`Scanning ${folder.path}`}
            className="h-1 overflow-hidden bg-surface-container-high"
          >
            <div className="h-full w-1/3 bg-primary motion-safe:animate-indeterminate" />
          </div>
          <span>
            <Button size="sm" variant="ghost" onClick={sync.stopScan}>
              Stop <ActionKeyHint actionId="onboarding.stopScan" />
            </Button>
          </span>
        </div>
      );
    case 'done': {
      const matched = countOnboardingFolderMatches(repos, folder.path);
      return (
        <div className="flex flex-col gap-3">
          <div className="flex items-baseline justify-between gap-4 min-w-0">
            <FolderPath path={folder.path} />
            <span className="mono-text text-[11px] text-outline shrink-0">
              {matched} {matched === 1 ? 'repo' : 'repos'} matched
            </span>
          </div>
          {folder.partial && (
            <p className="text-xs text-on-surface-variant">
              The scan stopped early, so some clones may not be matched yet.
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            <ChooseFolderButton
              label="Choose another folder"
              onClick={sync.chooseFolder}
            />
            <Button size="sm" variant="ghost" onClick={sync.scanAgain}>
              Scan again
            </Button>
          </div>
        </div>
      );
    }
    case 'error':
      return (
        <div className="flex flex-col gap-3">
          <p role="alert" className="flex gap-2 text-sm text-error">
            <span
              aria-hidden="true"
              className="material-symbols-outlined text-base"
            >
              error
            </span>
            <span className="min-w-0 break-words">
              Couldn't read {folder.path || 'that folder'}:{' '}
              {withoutTrailingPeriod(folder.reason)}. Repos will clone on first
              open instead.
            </span>
          </p>
          <span>
            <ChooseFolderButton
              label="Choose another folder"
              onClick={sync.chooseFolder}
            />
          </span>
        </div>
      );
    default: {
      const unhandled: never = folder;
      return unhandled;
    }
  }
}

/** Step 1: what to sync, plus the optional reuse-local-clones folder. */
export function OnboardingDevFolderStep({
  sync,
}: {
  readonly sync: OnboardingSync;
}): ReactElement {
  const { state } = sync;
  return (
    <div className="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)] gap-6 items-stretch grow">
      <section className="self-start bg-surface-container-lowest border border-outline-variant/30">
        <div className="p-6">
          <span
            aria-hidden="true"
            className="material-symbols-outlined flex w-10 h-10 items-center justify-center bg-surface-container-low border border-outline-variant/30 text-on-surface-variant mb-6"
          >
            inbox
          </span>
          <h2 className="text-xl font-bold text-on-surface mb-3">
            Start from your review queue
          </h2>
          <p className="text-sm text-on-surface-variant leading-relaxed mb-4">
            Bistro asks GitHub which PRs are waiting on you. Repos with requests
            start switched on, and repos you already sync stay on.
          </p>
          <p className="mono-text text-[11px] text-on-surface-variant leading-relaxed bg-surface-container-low p-3">
            Bistro only fetches into{' '}
            <code className="text-on-surface">refs/bistro/*</code>. It never
            touches your branches, tags, or files. Repos can be removed later.
          </p>
        </div>
        <div className="p-6 border-t border-outline-variant/30">
          <h3 className={`${LABEL} text-outline mb-3`}>
            {'Reuse local clones // optional'}
          </h3>
          <p className="text-sm text-on-surface-variant leading-relaxed mb-4">
            Point Bistro at your projects folder and it fetches into clones you
            already have instead of making new ones.
          </p>
          <FolderControls
            folder={state.folder}
            repos={state.selection.repos}
            sync={sync}
          />
        </div>
      </section>
      <OnboardingRepoTable state={state} />
    </div>
  );
}
