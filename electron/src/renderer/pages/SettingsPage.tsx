import { formatForDisplay } from '@tanstack/react-hotkeys';
import { useMemo, useState } from 'react';
import {
  CODE_FONT_CHOICES,
  type FontChoice,
  fontChoiceId,
  UI_FONT_CHOICES,
} from '../../types/settings';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { PROVIDERS } from '../providers';
import { appStore } from '../store';
import {
  openSettingsFile,
  resetSettings,
  settingsStore,
  updateSettings,
} from '../store/settings';
import type { UIState } from '../store/ui';
import { getEffectiveBindings, isSequenceBinding } from '../utils/keybindings';

type SettingsSection =
  | 'appearance'
  | 'keyboard'
  | 'connections'
  | 'sync'
  | 'advanced';

const SECTIONS: ReadonlyArray<{ id: SettingsSection; label: string }> = [
  { id: 'appearance', label: 'Appearance' },
  { id: 'keyboard', label: 'Keyboard' },
  { id: 'connections', label: 'Connections' },
  { id: 'sync', label: 'Sync' },
  { id: 'advanced', label: 'Advanced' },
];

/**
 * SettingsPage — matches `settings_minimal/code.html` with a left rail
 * sidebar and a stacked, scroll-spy-free content column.
 *
 * All mutations are routed through `updateSettings`, which writes the delta
 * through IPC to `settings.json`. The main process echoes the full settings
 * object back and our `useSettingsEffect` hook re-applies theme / font /
 * density CSS to the DOM.
 */
export function SettingsPage() {
  const [active, setActive] = useState<SettingsSection>('appearance');

  return (
    <main className="flex-1 overflow-y-auto bg-surface-container-lowest">
      <div className="max-w-6xl mx-auto px-8 md:px-12 lg:px-20 py-16 pb-40">
        <Header />
        <div className="grid grid-cols-1 md:grid-cols-12 gap-16">
          <Sidebar active={active} onSelect={setActive} />

          <div className="md:col-span-9 space-y-24">
            <AppearanceSection />
            <KeyboardSection />
            <ConnectionsSection />
            <SyncSection />
            <AdvancedSection />
          </div>
        </div>
      </div>
    </main>
  );
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

function Header() {
  return (
    <header className="mb-16">
      <div className="flex items-center gap-2 mb-4">
        <span className="font-mono text-[11px] tracking-widest text-primary uppercase">
          Workspace / Global
        </span>
      </div>
      <h1 className="text-5xl md:text-6xl font-black tracking-tighter mb-4 text-on-surface">
        Settings
      </h1>
      <p className="text-on-surface-variant font-mono text-[11px] tracking-widest uppercase border-l-2 border-primary pl-4">
        Technical Atelier / Preferences
      </p>
    </header>
  );
}

interface SidebarProps {
  active: SettingsSection;
  onSelect: (s: SettingsSection) => void;
}

function Sidebar({ active, onSelect }: SidebarProps) {
  return (
    <nav className="md:col-span-3 md:sticky md:top-8 md:self-start flex flex-col gap-1">
      <div className="mb-4 font-mono text-[10px] text-outline uppercase tracking-widest">
        Configuration
      </div>
      {SECTIONS.map((s) => {
        const isActive = s.id === active;
        return (
          <button
            key={s.id}
            type="button"
            onClick={() => {
              onSelect(s.id);
              const el = document.getElementById(`settings-section-${s.id}`);
              if (el !== null && typeof el.scrollIntoView === 'function') {
                el.scrollIntoView({ behavior: 'smooth', block: 'start' });
              }
            }}
            className={`text-left px-4 py-2 text-sm transition-colors border-l-2 ${
              isActive
                ? 'text-primary font-bold border-primary bg-surface-container-low'
                : 'text-on-surface-variant border-transparent hover:bg-surface-container-low'
            }`}
          >
            {s.label}
          </button>
        );
      })}
    </nav>
  );
}

// ---------------------------------------------------------------------------
// Appearance
// ---------------------------------------------------------------------------

function AppearanceSection() {
  const appearance = useStore(settingsStore, (s) => s.appearance);

  return (
    <Section id="appearance" title="Appearance">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-px bg-outline-variant/20 border border-outline-variant/20">
        <ThemeCard
          label="Light Mode"
          selected={appearance.theme === 'light'}
          variant="light"
          onSelect={() =>
            updateSettings({
              appearance: { ...appearance, theme: 'light' },
            })
          }
        />
        <ThemeCard
          label="Dark Mode"
          selected={appearance.theme === 'dark'}
          variant="dark"
          onSelect={() =>
            updateSettings({
              appearance: { ...appearance, theme: 'dark' },
            })
          }
        />
      </div>

      <RowToggle
        title="High Density View"
        description="Minimize whitespace for increased data density."
        on={appearance.highDensity}
        onChange={(on) =>
          updateSettings({ appearance: { ...appearance, highDensity: on } })
        }
      />

      <div className="grid grid-cols-1 md:grid-cols-2 gap-px bg-outline-variant/20 border border-outline-variant/20">
        <FontPicker
          label="UI Font"
          choices={UI_FONT_CHOICES}
          value={appearance.uiFont}
          onChange={(v) =>
            updateSettings({ appearance: { ...appearance, uiFont: v } })
          }
        />
        <FontPicker
          label="Code Font"
          choices={CODE_FONT_CHOICES}
          value={appearance.codeFont}
          onChange={(v) =>
            updateSettings({ appearance: { ...appearance, codeFont: v } })
          }
        />
        <NumberPicker
          label="UI Font Size"
          min={11}
          max={18}
          value={appearance.uiFontSize}
          onChange={(v) =>
            updateSettings({ appearance: { ...appearance, uiFontSize: v } })
          }
        />
        <NumberPicker
          label="Code Font Size"
          min={10}
          max={18}
          value={appearance.codeFontSize}
          onChange={(v) =>
            updateSettings({ appearance: { ...appearance, codeFontSize: v } })
          }
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
        <button
          type="button"
          onClick={() => onChange(clamp(value - 1))}
          className="w-10 h-10 border border-outline-variant hover:bg-surface-container-low transition-colors font-mono"
        >
          −
        </button>
        <input
          id={inputId}
          type="number"
          min={min}
          max={max}
          value={value}
          onChange={(e) => {
            const n = Number.parseInt(e.target.value, 10);
            if (!Number.isNaN(n)) onChange(clamp(n));
          }}
          className="w-full h-10 border-y border-outline-variant bg-surface text-center font-mono text-sm focus:outline-none focus:border-primary"
        />
        <button
          type="button"
          onClick={() => onChange(clamp(value + 1))}
          className="w-10 h-10 border border-outline-variant hover:bg-surface-container-low transition-colors font-mono"
        >
          +
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Keyboard
// ---------------------------------------------------------------------------

function KeyboardSection() {
  const overrides = useStore(settingsStore, (s) => s.keybindings);
  const groups = useMemo(() => {
    const bindings = getEffectiveBindings(overrides);
    const map = new Map<string, typeof bindings>();
    for (const b of bindings) {
      const modeKey =
        b.activation.routes?.[0] ?? b.activation.overlays?.[0] ?? 'global';
      const bucket = map.get(modeKey) ?? [];
      bucket.push(b);
      map.set(modeKey, bucket);
    }
    return Array.from(map.entries());
  }, [overrides]);

  return (
    <Section id="keyboard" title="Keyboard">
      <p className="font-mono text-[11px] text-on-surface-variant tracking-wide">
        Rebinding UI is coming in the next release. Until then, edit{' '}
        <button
          type="button"
          onClick={() => {
            void openSettingsFile();
          }}
          className="underline text-primary hover:opacity-80"
        >
          settings.json
        </button>{' '}
        directly under the <code>keybindings</code> key.
      </p>
      <div className="space-y-8">
        {groups.map(([mode, bindings]) => (
          <div key={mode}>
            <h3 className="font-mono text-[10px] font-bold text-outline tracking-widest uppercase mb-3 pb-2 border-b border-outline-variant/20">
              {mode}
            </h3>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-x-12 gap-y-2">
              {bindings.map((b, i) => (
                <div
                  key={`${mode}-${i}`}
                  className="flex items-center justify-between py-2 gap-4 border-b border-outline-variant/10 last:border-b-0"
                >
                  <span className="text-sm text-on-surface truncate">
                    {b.description}
                  </span>
                  <div className="flex items-center gap-1 shrink-0">
                    {(isSequenceBinding(b) ? Array.from(b.keys) : [b.keys as string]).map((k, j) => (
                      <kbd
                        key={`${k}-${j}`}
                        className="px-2 py-0.5 text-[10px] font-mono font-bold bg-surface-container-high text-on-surface border border-outline-variant/40"
                      >
                        {formatForDisplay(k as string)}
                      </kbd>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </Section>
  );
}

// ---------------------------------------------------------------------------
// Connections
// ---------------------------------------------------------------------------

function ConnectionsSection() {
  const connected = useStore(appStore, (s: UIState) => s.connectedProviders);
  const authLoading = useStore(appStore, (s: UIState) => s.authLoading);
  const { send } = useWebSocket();

  const handleConnect = (providerId: string) => {
    if (authLoading) return;
    appStore.setState((s) => ({
      ...s,
      authLoading: true,
      authError: null,
    }));
    send('AUTH_START', { provider: providerId });
  };

  const handleDisconnect = (providerId: string) => {
    send('AUTH_DISCONNECT', { provider: providerId });
  };

  return (
    <Section id="connections" title="Connections">
      <div className="grid grid-cols-1 gap-px bg-outline-variant/20 border border-outline-variant/20">
        {PROVIDERS.map((provider) => {
          const link = connected.find((c) => c.provider === provider.id);
          const isConnected = link !== undefined;
          return (
            <div
              key={provider.id}
              className="flex items-center justify-between p-6 md:p-8 bg-surface group hover:bg-surface-container-low transition-colors"
            >
              <div className="flex items-center gap-6">
                <div className="w-12 h-12 bg-slate-900 flex items-center justify-center">
                  <span className="material-symbols-outlined text-white text-[24px]">
                    {provider.icon}
                  </span>
                </div>
                <div>
                  <p className="font-bold text-base text-on-surface">
                    {provider.label}
                  </p>
                  <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
                    {isConnected
                      ? `Connected as @${link.username || 'unknown'}`
                      : 'Not connected'}
                  </p>
                </div>
              </div>
              {isConnected ? (
                <button
                  type="button"
                  onClick={() => handleDisconnect(provider.id)}
                  className="font-mono text-[10px] border border-outline px-5 py-2.5 hover:bg-slate-900 hover:text-white transition-colors uppercase tracking-widest"
                >
                  Disconnect
                </button>
              ) : (
                <button
                  type="button"
                  disabled={authLoading}
                  onClick={() => handleConnect(provider.id)}
                  className="font-mono text-[10px] bg-primary text-primary-foreground px-5 py-2.5 hover:opacity-90 disabled:opacity-50 transition-opacity uppercase tracking-widest"
                >
                  Connect
                </button>
              )}
            </div>
          );
        })}
      </div>
    </Section>
  );
}

// ---------------------------------------------------------------------------
// Sync
// ---------------------------------------------------------------------------

function SyncSection() {
  const sync = useStore(settingsStore, (s) => s.sync);

  return (
    <Section id="sync" title="Sync & Notifications">
      <div className="divide-y divide-outline-variant/10 border-y border-outline-variant/10">
        <RowToggle
          title="Auto Sync"
          description="Automatically refresh PRs in the background."
          on={sync.autoSync}
          onChange={(on) => updateSettings({ sync: { ...sync, autoSync: on } })}
          inline
        />
        <RowStepper
          title="Sync Interval"
          description="Seconds between auto-sync ticks."
          value={sync.syncInterval}
          min={30}
          max={3600}
          step={30}
          onChange={(v) =>
            updateSettings({ sync: { ...sync, syncInterval: v } })
          }
        />
        <RowToggle
          title="Notifications"
          description="Show toasts for new PRs and review requests."
          on={sync.notifications}
          onChange={(on) =>
            updateSettings({ sync: { ...sync, notifications: on } })
          }
          inline
        />
      </div>
    </Section>
  );
}

// ---------------------------------------------------------------------------
// Advanced
// ---------------------------------------------------------------------------

function AdvancedSection() {
  return (
    <Section id="advanced" title="Advanced">
      <div className="flex flex-wrap gap-3">
        <button
          type="button"
          onClick={() => {
            void openSettingsFile();
          }}
          className="font-mono text-[11px] uppercase tracking-widest px-5 py-3 bg-surface-container-high hover:bg-surface-container-highest transition-colors"
        >
          Open settings.json
        </button>
        <button
          type="button"
          onClick={() => {
            if (window.confirm('Reset all settings to defaults?') === true) {
              void resetSettings();
            }
          }}
          className="font-mono text-[11px] uppercase tracking-widest px-5 py-3 border border-error/50 text-error hover:bg-error/10 transition-colors"
        >
          Reset to Defaults
        </button>
      </div>
    </Section>
  );
}

// ---------------------------------------------------------------------------
// Shared primitives
// ---------------------------------------------------------------------------

interface SectionProps {
  id: string;
  title: string;
  children: React.ReactNode;
}

function Section({ id, title, children }: SectionProps) {
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

function RowToggle({
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
      <button
        type="button"
        role="switch"
        aria-checked={on}
        onClick={() => onChange(!on)}
        className={`relative w-12 h-6 flex items-center p-1 transition-colors ${
          on ? 'bg-primary' : 'bg-outline-variant'
        }`}
      >
        <div
          className={`w-4 h-4 bg-white transition-transform ${
            on ? 'ml-auto' : ''
          }`}
        />
      </button>
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

function RowStepper({
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
        <button
          type="button"
          onClick={() => onChange(clamp(value - step))}
          className="w-8 h-8 hover:bg-surface-container-low"
        >
          −
        </button>
        <span className="px-3 min-w-[3rem] text-center">{value}</span>
        <button
          type="button"
          onClick={() => onChange(clamp(value + step))}
          className="w-8 h-8 hover:bg-surface-container-low"
        >
          +
        </button>
      </div>
    </div>
  );
}
