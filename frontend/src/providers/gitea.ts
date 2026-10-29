import type { ProviderDef } from './provider';

export const gitea = {
  id: 'gitea',
  isAvailable: false,
  key: 't',
  label: 'Gitea',
  icon: 'coffee',
} as const satisfies ProviderDef;
