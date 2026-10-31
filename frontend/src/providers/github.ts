import type { ProviderDef } from './provider';

export const github = {
  id: 'github',
  isAvailable: true,
  key: 'g',
  label: 'GitHub',
  icon: 'terminal',
} as const satisfies ProviderDef;
