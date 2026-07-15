/** Shape every OAuth provider must satisfy. */
export interface ProviderDef {
  readonly id: string;
  readonly key: string;
  readonly label: string;
  readonly icon: string;
}

const AUTH_ACTION_PREFIX = 'auth_' as const;

export function authAction(provider: ProviderDef): string {
  return `${AUTH_ACTION_PREFIX}${provider.id}`;
}
