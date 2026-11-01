import { z } from 'zod';

export type {
  AppearanceSettings,
  ClassificationSettings,
  DiffSettings,
  NavigationSettings,
  PRListSettings,
  PrivacySettings,
  RepositorySettings as RepositoriesSettings,
  Settings as AppSettings,
  SyncSettings,
} from '@bindings/shell/models';

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
  keybindings: z
    .record(z.string(), z.array(z.string()).nullable().optional())
    .nullable()
    .transform((bindings) =>
      Object.fromEntries(
        Object.entries(bindings ?? {}).flatMap(([key, keys]) =>
          keys == null ? [] : [[key, keys]],
        ),
      ),
    ),
  window: z.object({
    x: z.number(),
    y: z.number(),
    width: z.number(),
    height: z.number(),
    maximised: z.boolean(),
  }),
});

export type KeybindingOverrides = z.infer<
  typeof appSettingsSchema
>['keybindings'];

export type DisplaySettings = z.infer<typeof appSettingsSchema>;

export const DEFAULT_SETTINGS: DisplaySettings = {
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
  window: { x: 0, y: 0, width: 0, height: 0, maximised: false },
};
