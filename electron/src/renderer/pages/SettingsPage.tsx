import { formatForDisplay } from '@tanstack/react-hotkeys';
import { err, fromPromise, ok, type Result, ResultAsync } from 'neverthrow';
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  type ClassificationMode,
  type ClassificationSettings,
  CODE_FONT_CHOICES,
  type DismissalScope,
  type FontChoice,
  fontChoiceId,
  UI_FONT_CHOICES,
} from '../../types/settings';
import { CacheUsageCard } from '../components/CacheUsageCard';
import { addToast } from '../components/ToastContainer';
import { Button } from '../components/ui/button';
import { Input } from '../components/ui/input';
import { Kbd } from '../components/ui/kbd';
import { Switch } from '../components/ui/switch';
import { Textarea } from '../components/ui/textarea';
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
import type { ExpertiseSettingsPayload } from '../types';
import type { LocationErrorCode } from '../types/repoLocation';
import { getEffectiveBindings, isSequenceBinding } from '../utils/keybindings';

type SettingsSection =
  | 'appearance'
  | 'keyboard'
  | 'connections'
  | 'repositories'
  | 'sync'
  | 'classification'
  | 'expertise'
  | 'privacy'
  | 'advanced';

const DEFAULT_MIN_SURVIVING_LINES = 50;

const SECTIONS: ReadonlyArray<{ id: SettingsSection; label: string }> = [
  { id: 'appearance', label: 'Appearance' },
  { id: 'keyboard', label: 'Keyboard' },
  { id: 'connections', label: 'Connections' },
  { id: 'repositories', label: 'Repositories' },
  { id: 'sync', label: 'Sync' },
  { id: 'classification', label: 'Comment Classification' },
  { id: 'expertise', label: 'Expertise' },
  { id: 'privacy', label: 'Privacy' },
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
            <RepositoriesSection />
            <SyncSection />
            <ClassificationSection />
            {active === 'expertise' && <ExpertiseSection />}
            <PrivacySection />
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
        <Button
          variant="link"
          size="sm"
          onClick={() => {
            void openSettingsFile();
          }}
          className="h-auto px-0 py-0 align-baseline normal-case tracking-normal"
        >
          settings.json
        </Button>{' '}
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
                    {(isSequenceBinding(b) ? b.keys : [b.keys as string]).map(
                      (k, j) => (
                        <Kbd
                          key={`${k}-${j}`}
                          className="px-2 text-[10px] font-bold text-on-surface"
                        >
                          {formatForDisplay(k)}
                        </Kbd>
                      ),
                    )}
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
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handleDisconnect(provider.id)}
                  className="px-5"
                >
                  Disconnect
                </Button>
              ) : (
                <Button
                  size="sm"
                  disabled={authLoading}
                  onClick={() => handleConnect(provider.id)}
                  className="px-5"
                >
                  Connect
                </Button>
              )}
            </div>
          );
        })}
      </div>
    </Section>
  );
}

// ---------------------------------------------------------------------------
// Repositories
// ---------------------------------------------------------------------------

interface ValidateDevFolderResponse {
  ok: true;
  canonical_path: string;
}

interface LocationErrorResponse {
  ok?: false;
  repo_id: string;
  operation: string;
  code: LocationErrorCode;
  message: string;
}

interface ScanDevFolderSummary {
  ok: true;
  discovered: number;
  healthy: number;
  degraded: number;
  skipped: number;
  partial: boolean;
}

const LOCATION_ERROR_CODES = [
  'path_missing',
  'remote_mismatch',
  'permission_denied',
  'invalid_url',
  'invalid_path',
  'destination_required',
  'destination_conflict',
  'clone_failed',
  'auth_failed',
  'cancelled',
  'delete_failed',
  'scan_partial',
] as const satisfies readonly LocationErrorCode[];

function isRecord(data: unknown): data is Record<string, unknown> {
  return typeof data === 'object' && data !== null;
}

function isLocationErrorCode(code: unknown): code is LocationErrorCode {
  return LOCATION_ERROR_CODES.includes(
    code as (typeof LOCATION_ERROR_CODES)[number],
  );
}

function parseValidateDevFolderResponse(
  data: unknown,
): Result<ValidateDevFolderResponse, string> {
  if (
    isRecord(data) &&
    data.ok === true &&
    typeof data.canonical_path === 'string'
  ) {
    return ok({ ok: true, canonical_path: data.canonical_path });
  }

  return err('Dev folder validation returned an invalid response.');
}

function parseScanDevFolderSummary(
  data: unknown,
): Result<ScanDevFolderSummary, string> {
  if (
    isRecord(data) &&
    data.ok === true &&
    typeof data.discovered === 'number' &&
    typeof data.healthy === 'number' &&
    typeof data.degraded === 'number' &&
    typeof data.skipped === 'number' &&
    typeof data.partial === 'boolean'
  ) {
    return ok({
      ok: true,
      discovered: data.discovered,
      healthy: data.healthy,
      degraded: data.degraded,
      skipped: data.skipped,
      partial: data.partial,
    });
  }

  return err('Scan failed.');
}

function isLocationErrorResponse(data: unknown): data is LocationErrorResponse {
  return (
    isRecord(data) &&
    data.ok !== true &&
    typeof data.repo_id === 'string' &&
    typeof data.operation === 'string' &&
    isLocationErrorCode(data.code) &&
    typeof data.message === 'string'
  );
}

function scanStatusText(scan: ScanDevFolderSummary | null): string {
  if (scan === null) {
    return 'No scan has run yet.';
  }

  const partial = scan.partial ? ' Partial scan; check the logs.' : '';
  return `${scan.discovered} discovered, ${scan.healthy} healthy, ${scan.degraded} degraded, ${scan.skipped} skipped.${partial}`;
}

function ensurePromise(value: Promise<unknown> | unknown): Promise<unknown> {
  return value instanceof Promise ? value : Promise.resolve(value);
}

function requestErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== '') {
    return error.message;
  }

  if (typeof error === 'string' && error.trim() !== '') {
    return error;
  }

  return 'Bistro could not reach the sync engine. Try again.';
}

function pickFolderResult(): ResultAsync<string | null, string> {
  if (window.electron?.pickFolder === undefined) {
    return ResultAsync.fromSafePromise(
      Promise.resolve<string | null>(null),
    ).andThen(() => err('Folder picker is unavailable.'));
  }

  return ResultAsync.fromPromise(
    window.electron.pickFolder(),
    () => 'Dev folder could not be changed.',
  );
}

function RepositoriesSection() {
  const repositories = useStore(settingsStore, (s) => s.repositories);
  const { request } = useWebSocket();
  const [scanResult, setScanResult] = useState<ScanDevFolderSummary | null>(
    null,
  );
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isChanging, setIsChanging] = useState(false);
  const currentPath = repositories.devFolderPath;

  const scanDevFolder = useCallback(
    (path: string): ResultAsync<ScanDevFolderSummary, string> =>
      ResultAsync.fromPromise(
        ensurePromise(request('SCAN_DEV_FOLDER', { path, reason: 'settings' })),
        requestErrorMessage,
      )
        .andThen(parseScanDevFolderSummary)
        .map((summary) => {
          setScanResult(summary);
          setErrorMessage(null);
          return summary;
        })
        .mapErr((message) => {
          setErrorMessage(message);
          return message;
        }),
    [request],
  );

  const handleChange = (): void => {
    setIsChanging(true);
    pickFolderResult()
      .andThen((path) => {
        if (path === null) {
          return ResultAsync.fromSafePromise(Promise.resolve(null));
        }

        return ResultAsync.fromPromise(
          ensurePromise(request('VALIDATE_DEV_FOLDER', { path })),
          requestErrorMessage,
        )
          .andThen((response) => {
            if (isLocationErrorResponse(response)) {
              return err(response.message);
            }

            return parseValidateDevFolderResponse(response);
          })
          .andThen((response: ValidateDevFolderResponse) =>
            updateSettings({
              repositories: {
                ...repositories,
                devFolderPath: response.canonical_path,
              },
            })
              .mapErr(() => 'Dev folder could not be changed.')
              .map(() => response.canonical_path),
          )
          .andThen(scanDevFolder)
          .map(() => null);
      })
      .match(
        () => setIsChanging(false),
        (message) => {
          setErrorMessage(message);
          addToast({ type: 'error', message, dismissible: true });
          setIsChanging(false);
        },
      );
  };

  const handleClear = (): void => {
    if (isChanging) {
      return;
    }
    setIsChanging(true);
    updateSettings({
      repositories: { ...repositories, devFolderPath: null },
    })
      .mapErr(() => 'Dev folder could not be cleared.')
      .match(
        () => {
          setScanResult(null);
          setErrorMessage(null);
          setIsChanging(false);
        },
        (message) => {
          setErrorMessage(message);
          addToast({ type: 'error', message, dismissible: true });
          setIsChanging(false);
        },
      );
  };

  return (
    <Section id="repositories" title="Repositories">
      <div className="space-y-6">
        <div className="border border-outline-variant/20 bg-surface p-6 md:p-8 space-y-4">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
            <div>
              <p className="font-bold text-sm text-on-surface">Dev folder</p>
              <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
                {currentPath ?? 'Not set'}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                size="sm"
                disabled={isChanging}
                onClick={handleChange}
              >
                Change
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={currentPath === null || isChanging}
                onClick={handleClear}
              >
                Clear
              </Button>
            </div>
          </div>
          <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
            {scanStatusText(scanResult)}
          </p>
          {errorMessage !== null && (
            <p role="alert" className="text-sm text-error">
              {errorMessage}
            </p>
          )}
        </div>
        <CacheUsageCard />
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
// Comment Classification
// ---------------------------------------------------------------------------

function ClassificationSection() {
  const classification = useStore(settingsStore, (s) => s.classification);
  const byokSessionTokens = useStore(
    appStore,
    (s: UIState) => s.byokSessionTokens,
  );
  const byokEstCost = useStore(appStore, (s: UIState) => s.byokEstCost);
  const byokAuthFailed = useStore(appStore, (s: UIState) => s.byokAuthFailed);
  const { request } = useWebSocket();
  const [keyInput, setKeyInput] = useState('');
  const [keySaved, setKeySaved] = useState(false);
  const [keySaving, setKeySaving] = useState(false);

  const setScope = (scope: DismissalScope): void => {
    void updateSettings({
      classification: { ...classification, dismissalScope: scope },
    });
  };

  const setMode = (mode: ClassificationMode): void => {
    void updateSettings({
      classification: { ...classification, mode },
    });
  };

  const isAIMode =
    classification.mode === 'rules+llm' || classification.mode === 'llm-all';

  const saveKey = (): void => {
    if (keyInput.length === 0 || keySaving) return;
    setKeySaving(true);
    // The key travels ONLY through SET_BYOK_KEY into the OS keychain —
    // never into settings JSON or any renderer store.
    void fromPromise(
      request('SET_BYOK_KEY', {
        provider: classification.provider,
        key: keyInput,
      }),
      (err) => err,
    ).match(
      () => {
        setKeySaving(false);
        setKeyInput('');
        setKeySaved(true);
        appStore.setState((s) => ({ ...s, byokAuthFailed: false }));
        addToast({
          type: 'success',
          message: `API key saved to the OS keychain (${classification.provider})`,
          dismissible: true,
        });
      },
      (err) => {
        // Key kept in the input so the user can retry.
        setKeySaving(false);
        addToast({
          type: 'error',
          message: `Saving key failed: ${err instanceof Error ? err.message : 'sync engine unreachable'}`,
          dismissible: true,
        });
      },
    );
  };

  // 'server' exists in the type but is a hidden Team-plan stub — never rendered.
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

  return (
    <Section id="classification" title="Comment Classification">
      <div className="space-y-4">
        <div>
          <h3 className="text-sm font-bold text-on-surface">
            Classification mode
          </h3>
          <p className="text-xs text-on-surface-variant mt-1">
            AI modes call your own LLM provider with your own key. Everything
            else stays on this machine.
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
                onClick={() => setMode(option.value)}
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

        {isAIMode && (
          <div className="space-y-3 border border-outline-variant/20 p-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <label className="block" htmlFor="byok-provider">
                <span className="block text-xs text-on-surface-variant mb-1">
                  Provider
                </span>
                <select
                  id="byok-provider"
                  value={classification.provider}
                  onChange={(e) =>
                    void updateSettings({
                      classification: {
                        ...classification,
                        provider: e.target
                          .value as ClassificationSettings['provider'],
                      },
                    })
                  }
                  className="w-full bg-surface-container-lowest border border-outline-variant/40 px-3 py-2 text-sm text-on-surface"
                >
                  <option value="anthropic">Anthropic</option>
                  <option value="openai">OpenAI</option>
                  <option value="openrouter">OpenRouter</option>
                  <option value="ollama">Ollama</option>
                  <option value="custom">Custom (OpenAI-compatible)</option>
                </select>
              </label>
              <label className="block" htmlFor="byok-model">
                <span className="block text-xs text-on-surface-variant mb-1">
                  Model
                </span>
                <Input
                  id="byok-model"
                  value={classification.model}
                  placeholder="e.g. claude-haiku-4-5"
                  onChange={(e) =>
                    void updateSettings({
                      classification: {
                        ...classification,
                        model: e.target.value,
                      },
                    })
                  }
                />
              </label>
            </div>
            {(classification.provider === 'custom' ||
              classification.provider === 'ollama') && (
              <label className="block" htmlFor="byok-endpoint">
                <span className="block text-xs text-on-surface-variant mb-1">
                  Endpoint
                </span>
                <Input
                  id="byok-endpoint"
                  value={classification.endpoint}
                  placeholder="http://localhost:11434/v1"
                  onChange={(e) =>
                    void updateSettings({
                      classification: {
                        ...classification,
                        endpoint: e.target.value,
                      },
                    })
                  }
                />
              </label>
            )}
            <div className="flex items-end gap-3">
              <label className="block flex-1" htmlFor="byok-key">
                <span
                  className={`block text-xs mb-1 ${
                    byokAuthFailed ? 'text-error' : 'text-on-surface-variant'
                  }`}
                >
                  API key{' '}
                  {byokAuthFailed
                    ? '— rejected by provider'
                    : '(stored in the OS keychain, never in settings)'}
                </span>
                <Input
                  id="byok-key"
                  type="password"
                  value={keyInput}
                  placeholder={keySaved ? '••••••••  (key saved)' : 'sk-…'}
                  onChange={(e) => setKeyInput(e.target.value)}
                  className={byokAuthFailed ? 'border-error' : ''}
                />
              </label>
              <Button
                onClick={saveKey}
                disabled={keyInput.length === 0 || keySaving}
                className="px-5 py-2"
              >
                {keySaving ? 'Saving…' : 'Save key'}
              </Button>
            </div>
            <p className="font-mono text-[10px] text-outline">
              This session: {byokSessionTokens.toLocaleString()} tokens, est. $
              {byokEstCost.toFixed(2)}
            </p>
          </div>
        )}

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
          {(
            [
              {
                value: 'local' as const,
                label: 'Local only',
                description: 'Dismissals stay on this machine.',
              },
              {
                value: 'local+provider' as const,
                label: 'Also resolve on GitHub',
                description:
                  'Resolvable threads are resolved on GitHub when dismissed.',
              },
            ] satisfies ReadonlyArray<{
              value: DismissalScope;
              label: string;
              description: string;
            }>
          ).map((option) => {
            const selected = classification.dismissalScope === option.value;
            return (
              <button
                key={option.value}
                type="button"
                role="radio"
                aria-checked={selected}
                onClick={() => setScope(option.value)}
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
      </div>
    </Section>
  );
}

// ---------------------------------------------------------------------------
// Expertise
// ---------------------------------------------------------------------------

function splitListInput(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
}

function joinListInput(values: readonly string[]): string {
  return values.join('\n');
}

export function expertiseSettingsPayloadFromInputs(
  aliasInput: string,
  minSurvivingLines: number,
  excludeInputs: Record<string, string>,
): ExpertiseSettingsPayload {
  return {
    alias_emails: splitListInput(aliasInput),
    min_surviving_lines: minSurvivingLines,
    excludes: Object.fromEntries(
      Object.entries(excludeInputs).map(
        ([repoId, value]) => [repoId, splitListInput(value)] as const,
      ),
    ),
  };
}

function settingsOrDefault(
  settings: ExpertiseSettingsPayload | null,
): ExpertiseSettingsPayload {
  return {
    alias_emails: settings?.alias_emails ?? [],
    min_surviving_lines:
      settings?.min_surviving_lines ?? DEFAULT_MIN_SURVIVING_LINES,
    excludes: settings?.excludes ?? {},
  };
}

function ExpertiseSection() {
  const { request } = useWebSocket();
  const repos = useStore(appStore, (s: UIState) => s.repos);
  const storedSettings = useStore(
    appStore,
    (s: UIState) => s.expertiseSettings,
  );
  const settings = settingsOrDefault(storedSettings);
  const [aliasInput, setAliasInput] = useState(
    joinListInput(settings.alias_emails),
  );
  const [minLinesInput, setMinLinesInput] = useState(
    String(settings.min_surviving_lines),
  );
  const [excludeInputs, setExcludeInputs] = useState<Record<string, string>>(
    () =>
      Object.fromEntries(
        Object.entries(settings.excludes).map(([repoId, globs]) => [
          repoId,
          joinListInput(globs),
        ]),
      ),
  );
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    void fromPromise(request('GET_EXPERTISE_SETTINGS', {}), (err) => err).match(
      () => undefined,
      (err) => {
        addToast({
          type: 'error',
          message: `Loading expertise settings failed: ${err instanceof Error ? err.message : 'sync engine unreachable'}`,
          dismissible: true,
        });
      },
    );
  }, [request]);

  useEffect(() => {
    if (storedSettings === null) return;
    setAliasInput(joinListInput(storedSettings.alias_emails));
    setMinLinesInput(String(storedSettings.min_surviving_lines));
    setExcludeInputs(
      Object.fromEntries(
        Object.entries(storedSettings.excludes).map(([repoId, globs]) => [
          repoId,
          joinListInput(globs),
        ]),
      ),
    );
  }, [storedSettings]);

  const watchedRepos = repos.filter((repo) => repo.watched);

  const save = (): void => {
    const minLines = Number.parseInt(minLinesInput, 10);
    if (!Number.isFinite(minLines) || minLines <= 0) {
      addToast({
        type: 'error',
        message: 'Minimum surviving lines must be greater than zero.',
        dismissible: true,
      });
      return;
    }

    const payload = expertiseSettingsPayloadFromInputs(
      aliasInput,
      minLines,
      excludeInputs,
    );

    setIsSaving(true);
    void fromPromise(
      request('SET_EXPERTISE_SETTINGS', payload),
      (err) => err,
    ).match(
      () => {
        setIsSaving(false);
        appStore.setState((s) => ({ ...s, expertiseSettings: payload }));
        addToast({
          type: 'success',
          message: 'Expertise settings saved.',
          dismissible: true,
        });
      },
      (err) => {
        setIsSaving(false);
        addToast({
          type: 'error',
          message: `Saving expertise settings failed: ${err instanceof Error ? err.message : 'sync engine unreachable'}`,
          dismissible: true,
        });
      },
    );
  };

  const setRepoExcludes = (repoId: string, value: string): void => {
    setExcludeInputs((current) => ({ ...current, [repoId]: value }));
  };

  return (
    <Section id="expertise" title="Expertise">
      <div className="space-y-6">
        <div>
          <h3 className="text-sm font-bold text-on-surface">
            Local expertise scoring
          </h3>
          <p className="text-xs text-on-surface-variant mt-1">
            Per-repo, opt-in indexing of your local git history. All scores stay
            in SQLite on this machine.
          </p>
        </div>

        <label className="block" htmlFor="expertise-alias-emails">
          <span className="block text-xs text-on-surface-variant mb-1">
            Alias emails
          </span>
          <Textarea
            id="expertise-alias-emails"
            value={aliasInput}
            rows={4}
            placeholder="you@company.com\n12345+you@users.noreply.github.com"
            onChange={(e) => setAliasInput(e.target.value)}
          />
          <span className="mt-1 block font-mono text-[10px] uppercase tracking-tight text-outline">
            One per line or comma-separated; counted as you for expertise
            matches.
          </span>
        </label>

        <label className="block max-w-xs" htmlFor="expertise-min-lines">
          <span className="block text-xs text-on-surface-variant mb-1">
            Minimum surviving lines
          </span>
          <Input
            id="expertise-min-lines"
            type="number"
            min={1}
            value={minLinesInput}
            onChange={(e) => setMinLinesInput(e.target.value)}
          />
        </label>

        <div className="space-y-3">
          <div>
            <h3 className="text-sm font-bold text-on-surface">
              Per-repo exclude globs
            </h3>
            <p className="text-xs text-on-surface-variant mt-1">
              Add generated or vendored paths to skip during indexing.
            </p>
          </div>
          {watchedRepos.length === 0 ? (
            <p className="font-mono text-[11px] text-outline uppercase tracking-widest">
              No watched repositories.
            </p>
          ) : (
            <div className="space-y-3">
              {watchedRepos.map((repo) => (
                <label
                  key={repo.id}
                  className="block"
                  htmlFor={`expertise-excludes-${repo.id}`}
                >
                  <span className="block text-xs text-on-surface-variant mb-1">
                    {repo.owner}/{repo.repo}
                  </span>
                  <Textarea
                    id={`expertise-excludes-${repo.id}`}
                    value={excludeInputs[repo.id] ?? ''}
                    rows={3}
                    placeholder="website/public/**\ngenerated/**"
                    onChange={(e) => setRepoExcludes(repo.id, e.target.value)}
                  />
                </label>
              ))}
            </div>
          )}
        </div>

        <div className="flex justify-end">
          <Button type="button" onClick={save} disabled={isSaving}>
            {isSaving ? 'Saving…' : 'Save expertise settings'}
          </Button>
        </div>
      </div>
    </Section>
  );
}

// ---------------------------------------------------------------------------
// Privacy
// ---------------------------------------------------------------------------

function PrivacySection() {
  const privacy = useStore(settingsStore, (s) => s.privacy);

  return (
    <Section id="privacy" title="Privacy">
      <div className="divide-y divide-outline-variant/10 border-y border-outline-variant/10">
        <RowToggle
          title="Usage Telemetry"
          description="Opt in to anonymous product events. No repo names, usernames, file paths, PR text, code, diffs, comments, or tokens are sent."
          on={privacy.telemetryEnabled}
          onChange={(on) =>
            updateSettings({
              privacy: { ...privacy, telemetryEnabled: on },
            })
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
        <Button
          variant="secondary"
          onClick={() => {
            void openSettingsFile();
          }}
          className="px-5 py-3"
        >
          Open settings.json
        </Button>
        <Button
          variant="destructive"
          onClick={() => {
            if (window.confirm('Reset all settings to defaults?') === true) {
              void resetSettings();
            }
          }}
          className="px-5 py-3"
        >
          Reset to Defaults
        </Button>
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
