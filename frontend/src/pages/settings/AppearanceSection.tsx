import {
  CODE_FONT_CHOICES,
  type FontChoice,
  fontChoiceId,
  UI_FONT_CHOICES,
} from '@renderer/utils/fontChoices';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { useStore } from '../../hooks/useStore';
import { settingsStore } from '../../store/settings';
import { RowToggle, Section, saveSettings } from './SettingsComponents';

export function AppearanceSection() {
  const appearance = useStore(settingsStore, (s) => s.appearance);
  const prList = useStore(settingsStore, (s) => s.prList);

  return (
    <Section id="appearance" title="Appearance">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-px bg-outline-variant/20 border border-outline-variant/20">
        <ThemeCard
          label="Light Mode"
          selected={appearance.theme === 'light'}
          variant="light"
          onSelect={() => saveSettings({ appearance: { theme: 'light' } })}
        />
        <ThemeCard
          label="Dark Mode"
          selected={appearance.theme === 'dark'}
          variant="dark"
          onSelect={() => saveSettings({ appearance: { theme: 'dark' } })}
        />
      </div>

      <RowToggle
        title="High Density View"
        description="Minimize whitespace for increased data density."
        on={appearance.highDensity}
        onChange={(on) => saveSettings({ appearance: { highDensity: on } })}
      />

      <div className="border border-outline-variant/20 bg-surface p-6">
        <label
          htmlFor="pr-list-view"
          className="block font-bold text-sm text-on-surface mb-1"
        >
          PR list view
        </label>
        <p className="font-mono text-[11px] text-on-surface-variant mb-4">
          Organize the inbox by review lanes or repository groups.
        </p>
        <select
          id="pr-list-view"
          value={prList.view}
          onChange={(event) =>
            saveSettings({
              prList: {
                view: event.target.value === 'grouped' ? 'grouped' : 'flat',
              },
            })
          }
          className="bg-surface border border-outline-variant focus:border-primary focus:outline-none transition-colors p-3 text-sm text-on-surface"
        >
          <option value="flat">Lanes</option>
          <option value="grouped">Repo groups</option>
        </select>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-px bg-outline-variant/20 border border-outline-variant/20">
        <FontPicker
          label="UI Font"
          choices={UI_FONT_CHOICES}
          value={appearance.uiFont}
          onChange={(v) => saveSettings({ appearance: { uiFont: v } })}
        />
        <FontPicker
          label="Code Font"
          choices={CODE_FONT_CHOICES}
          value={appearance.codeFont}
          onChange={(v) => saveSettings({ appearance: { codeFont: v } })}
        />
        <NumberPicker
          label="UI Font Size"
          min={11}
          max={18}
          value={appearance.uiFontSize}
          onChange={(v) => saveSettings({ appearance: { uiFontSize: v } })}
        />
        <NumberPicker
          label="Code Font Size"
          min={10}
          max={18}
          value={appearance.codeFontSize}
          onChange={(v) => saveSettings({ appearance: { codeFontSize: v } })}
        />
      </div>
    </Section>
  );
}

interface ThemeCardProps {
  label: string;
  selected: boolean;
  variant: 'light' | 'dark';
  onSelect: () => void;
}

function ThemeCard({ label, selected, variant, onSelect }: ThemeCardProps) {
  const base =
    variant === 'light'
      ? 'bg-white text-on-surface'
      : 'bg-slate-900 text-white';
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex flex-col gap-6 p-8 text-left group transition-opacity ${base} ${
        selected ? '' : 'opacity-70 hover:opacity-100'
      }`}
    >
      <div className="flex justify-between items-center w-full">
        <span className="font-bold text-sm">{label}</span>
        <span
          className={`material-symbols-outlined text-[20px] ${
            selected
              ? 'text-primary'
              : variant === 'dark'
                ? 'text-white/30'
                : 'text-outline'
          }`}
          style={{
            fontVariationSettings: selected ? "'FILL' 1" : "'FILL' 0",
          }}
        >
          {selected ? 'check_circle' : 'circle'}
        </span>
      </div>
      <div
        className={`w-full h-28 flex p-3 gap-3 border ${
          variant === 'light'
            ? 'bg-slate-50 border-slate-200'
            : 'bg-slate-800 border-white/10'
        }`}
      >
        <div
          className={`w-1/4 border ${
            variant === 'light'
              ? 'bg-white border-slate-200'
              : 'bg-slate-950 border-white/5'
          }`}
        />
        <div
          className={`w-3/4 border ${
            variant === 'light'
              ? 'bg-white border-slate-200'
              : 'bg-slate-950 border-white/5'
          }`}
        />
      </div>
    </button>
  );
}

interface FontPickerProps {
  label: string;
  choices: readonly FontChoice[];
  value: string;
  onChange: (v: string) => void;
}

function FontPicker({ label, choices, value, onChange }: FontPickerProps) {
  const currentId = fontChoiceId(choices, value);
  const selectId = `font-picker-${label.replace(/\s+/g, '-').toLowerCase()}`;
  return (
    <div className="p-6 bg-surface">
      <label
        htmlFor={selectId}
        className="block font-mono text-[10px] text-outline uppercase tracking-widest mb-3"
      >
        {label}
      </label>
      <select
        id={selectId}
        value={currentId ?? ''}
        onChange={(e) => {
          const next = choices.find((c) => c.id === e.target.value);
          if (next !== undefined) onChange(next.value);
        }}
        className="w-full bg-surface border border-outline-variant focus:border-primary focus:outline-none transition-colors p-3 text-sm text-on-surface"
      >
        {currentId === null && (
          <option value="" disabled>
            Custom ({value.split(',')[0]?.replace(/"/g, '') ?? 'custom'})
          </option>
        )}
        {choices.map((c) => (
          <option key={c.id} value={c.id}>
            {c.label}
          </option>
        ))}
      </select>
    </div>
  );
}

interface NumberPickerProps {
  label: string;
  min: number;
  max: number;
  value: number;
  onChange: (v: number) => void;
}

function NumberPicker({ label, min, max, value, onChange }: NumberPickerProps) {
  const clamp = (v: number) => Math.min(max, Math.max(min, v));
  const inputId = `number-picker-${label.replace(/\s+/g, '-').toLowerCase()}`;
  return (
    <div className="p-6 bg-surface">
      <label
        htmlFor={inputId}
        className="block font-mono text-[10px] text-outline uppercase tracking-widest mb-3"
      >
        {label}
      </label>
      <div className="flex items-center">
        <Button
          variant="outline"
          size="icon"
          aria-label={`Decrease ${label}`}
          onClick={() => onChange(clamp(value - 1))}
          className="h-10 w-10 border-outline-variant"
        >
          −
        </Button>
        <Input
          id={inputId}
          type="number"
          min={min}
          max={max}
          value={value}
          onChange={(e) => {
            const n = Number.parseInt(e.target.value, 10);
            if (!Number.isNaN(n)) onChange(clamp(n));
          }}
          className="h-10 border-x-0 border-y-outline-variant text-center [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
        />
        <Button
          variant="outline"
          size="icon"
          aria-label={`Increase ${label}`}
          onClick={() => onChange(clamp(value + 1))}
          className="h-10 w-10 border-outline-variant"
        >
          +
        </Button>
      </div>
    </div>
  );
}
