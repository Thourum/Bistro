import type { ProviderDef } from './provider';

export const gitlab = {
  id: 'gitlab',
  key: 'l',
  label: 'GitLab',
  icon: 'source',
} as const satisfies ProviderDef;
