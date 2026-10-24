/** Shape every OAuth provider must satisfy. */
export interface ProviderDef {
  readonly id: string;
  readonly key: string;
  readonly label: string;
  readonly icon: string;
  readonly isAvailable: boolean;
}

const AUTH_ACTION_PREFIX = 'auth_' as const;

export type AuthAction = `${typeof AUTH_ACTION_PREFIX}${string}`;

export function authAction(provider: ProviderDef): AuthAction {
  return `${AUTH_ACTION_PREFIX}${provider.id}`;
}
