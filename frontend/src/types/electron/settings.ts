/**
 * AppSettings — the full user-configurable state persisted to
 * `userData/settings.json` by the main process, and mirrored into the
 * renderer via `window.electron.settings.*`.
 *
 * Keep this file framework-agnostic: it's imported by both main and
 * renderer. Bumping `$schema` requires a migration.
 */

import { z } from 'zod';

export const SETTINGS_SCHEMA_VERSION = 1 as const;

const classificationSchema = z.object({
  mode: z.enum(['rules', 'rules+llm', 'llm-all', 'server']),
  dismissalScope: z.enum(['local', 'local+provider']),
  provider: z.enum(['anthropic', 'openai', 'openrouter', 'ollama', 'custom']),
  model: z.string(),
  endpoint: z.string(),
});

export const NAVIGATION_STYLES = ['vim', 'emacs', 'standard'] as const;
const navigationSchema = z.object({ style: z.enum(NAVIGATION_STYLES) });
export type NavigationStyle = (typeof NAVIGATION_STYLES)[number];
export type NavigationSettings = z.infer<typeof navigationSchema>;

const appearanceSchema = z.object({
  theme: z.enum(['light', 'dark']),
  uiFont: z.string(),
  codeFont: z.string(),
  uiFontSize: z.number().finite().positive(),
  codeFontSize: z.number().finite().positive(),
  highDensity: z.boolean(),
  onboardingComplete: z.boolean(),
  tourComplete: z.boolean(),
});

const syncSchema = z.object({
  autoSync: z.boolean(),
  syncInterval: z.number().finite().positive(),
  notifications: z.boolean(),
});

const prListSchema = z.object({ view: z.enum(['flat', 'grouped']) });
const diffSchema = z.object({ style: z.enum(['unified', 'split']) });
const privacySchema = z.object({ telemetryEnabled: z.boolean() });
const repositoriesSchema = z.object({ devFolderPath: z.string().nullable() });

export type ClassificationMode = z.infer<typeof classificationSchema>['mode'];
export type DismissalScope = z.infer<
  typeof classificationSchema
>['dismissalScope'];
export type ClassificationSettings = z.infer<typeof classificationSchema>;
export type AppearanceSettings = z.infer<typeof appearanceSchema>;
export type SyncSettings = z.infer<typeof syncSchema>;
export type PRListSettings = z.infer<typeof prListSchema>;
export type DiffSettings = z.infer<typeof diffSchema>;
export type PrivacySettings = z.infer<typeof privacySchema>;
export type RepositoriesSettings = z.infer<typeof repositoriesSchema>;

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
export const appSettingsSchema = z.object({
  $schema: z.literal(SETTINGS_SCHEMA_VERSION),
  appearance: appearanceSchema,
  sync: syncSchema,
  prList: prListSchema,
  diff: diffSchema,
  privacy: privacySchema,
  repositories: repositoriesSchema,
  classification: classificationSchema,
  navigation: navigationSchema,
  keybindings: z.record(z.string(), z.array(z.string())),
});

export type KeybindingOverrides = z.infer<
  typeof appSettingsSchema
>['keybindings'];
export type AppSettings = z.infer<typeof appSettingsSchema>;

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
    tourComplete: false,
  },
  sync: {
    autoSync: true,
    syncInterval: 60,
    notifications: true,
  },
  prList: {
    view: 'flat',
  },
  diff: {
    style: 'unified',
  },
  privacy: {
    telemetryEnabled: false,
  },
  repositories: {
    devFolderPath: null,
  },
  classification: {
    mode: 'rules',
    dismissalScope: 'local',
    provider: 'anthropic',
    model: '',
    endpoint: '',
  },
  navigation: { style: 'vim' },
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
