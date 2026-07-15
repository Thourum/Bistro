import type { ProviderDef } from './provider';

export type { ProviderDef } from './provider';
export { authAction } from './provider';

import { gitea } from './gitea';
import { github } from './github';
import { gitlab } from './gitlab';

/** All registered providers. Add new providers here. */
export const PROVIDERS = [
  github,
  gitlab,
  gitea,
] as const satisfies readonly ProviderDef[];

/** Union of all provider IDs, derived from the registry. */
export type ProviderId = (typeof PROVIDERS)[number]['id'];

/** Look up a provider by its action string (e.g. 'auth_github' → github def). */
export function parseAuthAction(action: string): ProviderDef | null {
  for (const provider of PROVIDERS) {
    if (action === `auth_${provider.id}`) return provider;
  }
  return null;
}
