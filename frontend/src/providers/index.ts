import type { ProviderDef } from './provider';

export type { ProviderDef } from './provider';

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
