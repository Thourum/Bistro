import type {
  ClassificationMode,
  ClassificationSettings,
  DismissalScope,
} from '@renderer/types/settings';
import { useStore } from '../../hooks/useStore';
import { settingsStore } from '../../store/settings';
import { ClassificationProvider } from './ClassificationProvider';
import { Section, saveSettings } from './SettingsComponents';

const MODE_OPTIONS = [
  {
    value: 'rules' as const,
    label: 'Rules engine',
    description: 'Deterministic YAML rules + heuristics. Fully offline.',
  },
  {
    value: 'rules+llm' as const,
    label: 'Rules + AI fallback',
    description: 'Your LLM classifies only what the rules left unclassified.',
  },
  {
    value: 'llm-all' as const,
    label: 'AI for everything',
    description:
      'Every comment goes to your LLM; rules shown until it answers.',
  },
] satisfies ReadonlyArray<{
  value: ClassificationMode;
  label: string;
  description: string;
}>;

const DISMISSAL_OPTIONS = [
  {
    value: 'local' as const,
    label: 'Local only',
    description: 'Dismissals stay on this machine.',
  },
  {
    value: 'local+provider' as const,
    label: 'Also resolve on GitHub',
    description: 'Resolvable threads are resolved on GitHub when dismissed.',
  },
] satisfies ReadonlyArray<{
  value: DismissalScope;
  label: string;
  description: string;
}>;

interface ClassificationChoicesProps {
  classification: ClassificationSettings;
}

function ClassificationChoices({ classification }: ClassificationChoicesProps) {
  return (
    <>
      <div>
        <h3 className="text-sm font-bold text-on-surface">
          Classification mode
        </h3>
        <p className="text-xs text-on-surface-variant mt-1">
          AI modes call your own LLM provider with your own key. Everything else
          stays on this machine.
        </p>
      </div>
      <div
        role="radiogroup"
        aria-label="Classification mode"
        className="divide-y divide-outline-variant/10 border-y border-outline-variant/10"
      >
        {MODE_OPTIONS.map((option) => {
          const selected = classification.mode === option.value;
          return (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={selected}
              onClick={() =>
                saveSettings({
                  classification: { mode: option.value },
                })
              }
              className={`flex w-full items-center justify-between gap-4 px-4 py-3 text-left transition-colors ${
                selected
                  ? 'bg-surface-container-low'
                  : 'hover:bg-surface-container-low/50'
              }`}
            >
              <span>
                <span className="block text-sm text-on-surface">
                  {option.label}
                </span>
                <span className="block text-xs text-on-surface-variant">
                  {option.description}
                </span>
              </span>
              <span
                className={`font-mono text-[10px] uppercase tracking-widest ${
                  selected ? 'text-primary font-bold' : 'text-outline'
                }`}
              >
                {selected ? '● Selected' : '○'}
              </span>
            </button>
          );
        })}
      </div>
    </>
  );
}

function DismissalChoices({ classification }: ClassificationChoicesProps) {
  return (
    <>
      <div>
        <h3 className="text-sm font-bold text-on-surface">Dismissal scope</h3>
        <p className="text-xs text-on-surface-variant mt-1">
          Bulk-dismissing comments always hides them locally. Optionally also
          resolve their review threads on GitHub via the action queue.
        </p>
      </div>
      <div
        role="radiogroup"
        aria-label="Dismissal scope"
        className="divide-y divide-outline-variant/10 border-y border-outline-variant/10"
      >
        {DISMISSAL_OPTIONS.map((option) => {
          const selected = classification.dismissalScope === option.value;
          return (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={selected}
              onClick={() =>
                saveSettings({
                  classification: {
                    dismissalScope: option.value,
                  },
                })
              }
              className={`flex w-full items-center justify-between gap-4 px-4 py-3 text-left transition-colors ${
                selected
                  ? 'bg-surface-container-low'
                  : 'hover:bg-surface-container-low/50'
              }`}
            >
              <span>
                <span className="block text-sm text-on-surface">
                  {option.label}
                </span>
                <span className="block text-xs text-on-surface-variant">
                  {option.description}
                </span>
              </span>
              <span
                className={`font-mono text-[10px] uppercase tracking-widest ${
                  selected ? 'text-primary font-bold' : 'text-outline'
                }`}
              >
                {selected ? '● Selected' : '○'}
              </span>
            </button>
          );
        })}
      </div>
    </>
  );
}

export function ClassificationSection() {
  const classification = useStore(settingsStore, (s) => s.classification);
  const isAIMode =
    classification.mode === 'rules+llm' || classification.mode === 'llm-all';

  return (
    <Section id="classification" title="Comment Classification">
      <div className="space-y-4">
        <ClassificationChoices classification={classification} />
        <ClassificationProvider
          classification={classification}
          isAIMode={isAIMode}
        />
        <DismissalChoices classification={classification} />
      </div>
    </Section>
  );
}
