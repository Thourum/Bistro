import type { ReactNode } from 'react';
import type { AppSettings } from '../../../types/settings';
import { addToast } from '../../components/ToastContainer';
import { Button } from '../../components/ui/button';
import { Switch } from '../../components/ui/switch';
import { updateSettings } from '../../store/settings';

export function showSettingsError(message: string): void {
  addToast({ type: 'error', message, dismissible: true });
}

export function saveSettings(patch: Partial<AppSettings>): void {
  void updateSettings(patch).match(() => undefined, showSettingsError);
}

interface SectionProps {
  id: string;
  title: string;
  children: ReactNode;
}

export function Section({ id, title, children }: SectionProps) {
  return (
    <section id={`settings-section-${id}`} className="space-y-8 scroll-mt-8">
      <div className="flex justify-between items-end border-b border-outline-variant/30 pb-4">
        <h2 className="text-3xl font-bold tracking-tight text-on-surface">
          {title}
        </h2>
      </div>
      {children}
    </section>
  );
}

interface RowToggleProps {
  title: string;
  description: string;
  on: boolean;
  onChange: (on: boolean) => void;
  inline?: boolean;
}

export function RowToggle({
  title,
  description,
  on,
  onChange,
  inline,
}: RowToggleProps) {
  return (
    <div
      className={`flex items-center justify-between ${
        inline ? 'py-6' : 'p-8 border border-outline-variant/20 bg-surface'
      }`}
    >
      <div>
        <p className="font-bold text-sm text-on-surface">{title}</p>
        <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
          {description}
        </p>
      </div>
      <Switch checked={on} onCheckedChange={onChange} aria-label={title} />
    </div>
  );
}

interface RowStepperProps {
  title: string;
  description: string;
  value: number;
  min: number;
  max: number;
  step?: number;
  onChange: (v: number) => void;
}

export function RowStepper({
  title,
  description,
  value,
  min,
  max,
  step = 1,
  onChange,
}: RowStepperProps) {
  const clamp = (v: number) => Math.min(max, Math.max(min, v));
  return (
    <div className="flex items-center justify-between py-6">
      <div>
        <p className="font-bold text-sm text-on-surface">{title}</p>
        <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
          {description}
        </p>
      </div>
      <div className="flex items-center border border-outline-variant font-mono text-sm">
        <Button
          variant="ghost"
          size="icon"
          aria-label={`Decrease ${title}`}
          onClick={() => onChange(clamp(value - step))}
          className="h-8 w-8"
        >
          −
        </Button>
        <span className="px-3 min-w-[3rem] text-center">{value}</span>
        <Button
          variant="ghost"
          size="icon"
          aria-label={`Increase ${title}`}
          onClick={() => onChange(clamp(value + step))}
          className="h-8 w-8"
        >
          +
        </Button>
      </div>
    </div>
  );
}
