/**
 * AppSettings — the full user-configurable state persisted to
 * `userData/settings.json` by the main process, and mirrored into the
 * renderer via `window.electron.settings.*`.
 *
 * Keep this file framework-agnostic: it's imported by both main and
 * renderer. Bumping `$schema` requires a migration.
 */

export const SETTINGS_SCHEMA_VERSION = 1 as const;

export type ThemeMode = 'light' | 'dark';

export interface AppearanceSettings {
  theme: ThemeMode;
  /** CSS font-family stack for all UI (not code). */
  uiFont: string;
  /** CSS font-family stack for code, diffs, kbd, metadata. */
  codeFont: string;
  /** Base UI font size in px. */
  uiFontSize: number;
  /** Monospace / code font size in px. */
  codeFontSize: number;
  /** Tightens vertical rhythm on list rows and cards. */
  highDensity: boolean;
  /** Set to true after the user completes (or skips) onboarding. */
  onboardingComplete: boolean;
}

export interface SyncSettings {
  autoSync: boolean;
  /** Seconds between auto-sync ticks. */
  syncInterval: number;
  notifications: boolean;
}

export interface PRListSettings {
  view: 'flat' | 'grouped';
}

/**
 * Overrides for keyboard bindings.
 *
 * Maps a stable `actionId` (see `utils/keybindings.ts`) to an array of
 * TanStack Hotkeys strings. One element = a single hotkey; multiple
 * elements = a Vim-style sequence; an empty array unbinds the action.
 * Absence means the default is used.
 *
 * Examples:
 *   { "list.down": ["j"] }          // single key
 *   { "list.top": ["g", "g"] }      // two-key sequence
 *   { "list.search": ["Meta+/"] }   // modifier combo
 *   { "list.mute": [] }             // unbind
 */
export type KeybindingOverrides = Record<string, string[]>;

export interface AppSettings {
  $schema: typeof SETTINGS_SCHEMA_VERSION;
  appearance: AppearanceSettings;
  sync: SyncSettings;
  prList: PRListSettings;
  keybindings: KeybindingOverrides;
}

export const DEFAULT_SETTINGS: AppSettings = {
  $schema: SETTINGS_SCHEMA_VERSION,
  appearance: {
    theme: 'light',
    uiFont: '"Geist Variable", "Geist", "Inter", system-ui, sans-serif',
    codeFont:
      '"JetBrains Mono", "Geist Mono Variable", "Fira Code", "IBM Plex Mono", monospace',
    uiFontSize: 14,
    codeFontSize: 13,
    highDensity: false,
    onboardingComplete: false,
  },
  sync: {
    autoSync: true,
    syncInterval: 60,
    notifications: true,
  },
  prList: {
    view: 'flat',
  },
  keybindings: {},
};

/**
 * Curated list of fonts available in the Appearance settings UI.
 *
 * Bundled via `@fontsource*` imports in `main.tsx`. Each entry's `value`
 * is the CSS font-family stack written to `--app-ui-font` / `--app-code-font`
 * at runtime.
 */
export interface FontChoice {
  id: string;
  label: string;
  value: string;
}

export const UI_FONT_CHOICES: readonly FontChoice[] = [
  {
    id: 'geist',
    label: 'Geist',
    value: '"Geist Variable", "Geist", "Inter", system-ui, sans-serif',
  },
  {
    id: 'inter',
    label: 'Inter',
    value: '"Inter", system-ui, sans-serif',
  },
  {
    id: 'ibm-plex-sans',
    label: 'IBM Plex Sans',
    value: '"IBM Plex Sans", system-ui, sans-serif',
  },
  {
    id: 'system-ui',
    label: 'System',
    value: 'system-ui, -apple-system, BlinkMacSystemFont, sans-serif',
  },
] as const;

export const CODE_FONT_CHOICES: readonly FontChoice[] = [
  {
    id: 'jetbrains-mono',
    label: 'JetBrains Mono',
    value: '"JetBrains Mono", monospace',
  },
  {
    id: 'geist-mono',
    label: 'Geist Mono',
    value: '"Geist Mono Variable", "Geist Mono", monospace',
  },
  {
    id: 'fira-code',
    label: 'Fira Code',
    value: '"Fira Code", monospace',
  },
  {
    id: 'ibm-plex-mono',
    label: 'IBM Plex Mono',
    value: '"IBM Plex Mono", monospace',
  },
] as const;

/** Helper: match a saved font stack to a curated `FontChoice` id. */
export function fontChoiceId(
  choices: readonly FontChoice[],
  value: string,
): string | null {
  const match = choices.find((c) => c.value === value);
  return match?.id ?? null;
}
