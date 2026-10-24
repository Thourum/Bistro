import type { ReactElement } from 'react';
import { Button } from '../components/ui/button';

interface OnboardingDevFolderStepProps {
  readonly discoveredCount: number | null;
  readonly onChooseFolder: () => void;
  readonly onSkip: () => void;
}

export function OnboardingDevFolderStep({
  discoveredCount,
  onChooseFolder,
  onSkip,
}: OnboardingDevFolderStepProps): ReactElement {
  return (
    <section className="bg-surface-container-lowest border border-outline-variant/30 p-8">
      <div className="flex items-center gap-3 mb-4">
        <span
          aria-hidden="true"
          className="material-symbols-outlined text-primary"
        >
          folder_open
        </span>
        <h2 className="mono-text text-xs font-bold uppercase tracking-widest">
          Use your existing projects folder
        </h2>
      </div>
      <p className="text-sm text-on-surface-variant leading-relaxed mb-3">
        Bistro registers every GitHub repo found in this folder and starts
        syncing PRs for healthy discoveries.
      </p>
      <p className="mono-text text-[10px] text-outline uppercase tracking-tight leading-normal mb-6">
        Bistro fetches PR objects into these repos only under refs/bistro/* and
        never touches branches, tags, or files. Unwanted repos can be removed
        afterwards via the repo recovery/remove flow.
      </p>
      {discoveredCount !== null && (
        <p className="text-sm text-primary font-medium mb-4">
          {discoveredCount}{' '}
          {discoveredCount === 1
            ? 'repository discovered'
            : 'repositories discovered'}
        </p>
      )}
      <div className="flex flex-wrap gap-3">
        <Button
          className="bg-primary! text-on-primary! border-primary! hover:bg-on-primary-fixed-variant!"
          onClick={onChooseFolder}
        >
          Choose folder
        </Button>
        <Button
          className="border-outline-variant!"
          variant="outline"
          onClick={onSkip}
        >
          Skip
        </Button>
      </div>
    </section>
  );
}
