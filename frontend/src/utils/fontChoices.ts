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
