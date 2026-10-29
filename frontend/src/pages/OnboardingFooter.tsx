import type { ReactElement } from 'react';
import { Button } from '../components/ui/button';
import { Kbd } from '../components/ui/kbd';
import { useBindingEnabled } from '../hooks/useAppHotkeys';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { findActionKeys } from '../utils/legendPresets';

type OnboardingStep = 'sync' | 'practice';
type StatusTone = 'neutral' | 'active' | 'success' | 'error';

interface FooterStatus {
  readonly label: string;
  readonly tone: StatusTone;
}

const STEP_NUMBER: Readonly<Record<OnboardingStep, number>> = {
  sync: 1,
  practice: 2,
};
const STEP_COUNT = 2;
const STEP_SEGMENTS = [1, 2] as const;

const SYNC_STATUS: Readonly<Record<UIState['syncStatus'], FooterStatus>> = {
  syncing: { label: 'SYNCING', tone: 'active' },
  ready: { label: 'SYNCED', tone: 'success' },
  connected: { label: 'WAITING TO SYNC', tone: 'neutral' },
  error: { label: 'SYNC ERROR', tone: 'error' },
  rate_limited: { label: 'RATE LIMITED', tone: 'error' },
};

const DOT_CLASS: Readonly<Record<StatusTone, string>> = {
  neutral: 'bg-outline',
  active: 'bg-yellow-500 animate-pulse motion-reduce:animate-none',
  success: 'bg-green-500',
  error: 'bg-error',
};

const LABEL_CLASS: Readonly<Record<StatusTone, string>> = {
  neutral: 'text-on-surface-variant',
  active: 'text-on-surface',
  success: 'text-on-surface',
  error: 'text-error',
};

interface OnboardingFooterProps {
  readonly step: OnboardingStep;
  readonly canContinue: boolean;
  readonly isFinishing: boolean;
  readonly isPracticeComplete: boolean;
  readonly onContinue: () => void;
}

/** Sum of review requests from a successful queue read, or null before one. */
function useNeedYouCount(): number | null {
  return useStore(appStore, (s) => {
    const sync = s.onboardingSync;
    if (sync === null || sync.queueError !== null) return null;
    const requested = sync.selection.queue?.requested ?? [];
    const total = requested.reduce((sum, request) => sum + request.count, 0);
    return total > 0 ? total : null;
  });
}

function OnboardingSyncStatus(): ReactElement {
  const syncStatus = useStore(appStore, (s) => s.syncStatus);
  const needYou = useNeedYouCount();
  const status = SYNC_STATUS[syncStatus];
  return (
    <p
      aria-live="polite"
      className="flex items-center gap-2 min-w-0 mono-text text-[10px] uppercase tracking-widest"
    >
      <span
        aria-hidden="true"
        className={`w-2 h-2 shrink-0 ${DOT_CLASS[status.tone]}`}
      />
      <span className={`truncate ${LABEL_CLASS[status.tone]}`}>
        {status.label}
        {needYou !== null && (
          <span className="text-on-surface-variant">{` // ${needYou} NEED YOU`}</span>
        )}
      </span>
    </p>
  );
}

function StepProgress({
  step,
}: {
  readonly step: OnboardingStep;
}): ReactElement {
  const current = STEP_NUMBER[step];
  return (
    <span className="flex items-center gap-3 shrink-0">
      <span className="mono-text text-xs text-on-surface">
        <span className="sr-only">Step </span>
        {`${current} / ${STEP_COUNT}`}
      </span>
      <span aria-hidden="true" className="flex gap-1">
        {STEP_SEGMENTS.map((segment) => (
          <span
            key={segment}
            className={`h-0.5 w-5 ${segment <= current ? 'bg-primary' : 'bg-outline-variant'}`}
          />
        ))}
      </span>
    </span>
  );
}

/** Pinned onboarding footer: progress, live sync status, and the primary action. */
export function OnboardingFooter({
  step,
  canContinue,
  isFinishing,
  isPracticeComplete,
  onContinue,
}: OnboardingFooterProps): ReactElement {
  const bindings = useEffectiveBindings();
  const continueBinding = bindings.find(
    (binding) =>
      binding.actionId === 'onboarding.continue' && binding.keys.length > 0,
  );
  const isKeyEnabled = useBindingEnabled(continueBinding);
  const continueKeys = findActionKeys(bindings, ['onboarding.continue']);
  const isFinish = step === 'practice';
  const highlight =
    isFinish && isPracticeComplete
      ? ' ring-2 ring-primary ring-offset-2 ring-offset-background'
      : '';

  return (
    <footer className="shrink-0 border-t border-outline-variant/30 bg-surface-container-lowest px-6 py-3 flex items-center justify-between gap-6">
      <div className="flex items-center gap-8 min-w-0">
        <StepProgress step={step} />
        <OnboardingSyncStatus />
      </div>
      <div className="flex items-center gap-3 shrink-0">
        {isKeyEnabled &&
          canContinue &&
          !isFinishing &&
          continueKeys !== undefined && (
            <span className="flex items-center gap-2 mono-text text-[10px] text-outline">
              or press <Kbd>{continueKeys}</Kbd>
            </span>
          )}
        <Button
          className={`bg-primary! text-on-primary! border-primary! hover:bg-on-primary-fixed-variant!${highlight}`}
          disabled={!canContinue || isFinishing}
          onClick={onContinue}
        >
          {isFinish ? 'Finish' : 'Continue'}
        </Button>
      </div>
    </footer>
  );
}
