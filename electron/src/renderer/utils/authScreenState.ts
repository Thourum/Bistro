export type AuthScreenState =
  | 'idle'
  | 'waiting'
  | 'timedOut'
  | 'failed'
  | 'interrupted'
  | 'engineDown';
export type FirstRunScreen =
  | AuthScreenState
  | 'connecting'
  | 'slow'
  | 'crashed';

export interface AuthScreenInput {
  readonly isConnected: boolean;
  readonly authLoading: boolean;
  readonly authUrl: string | null;
  readonly authError: string | null;
  readonly authTimedOut: boolean;
  readonly authInterrupted: boolean;
}

export function getAuthScreenState(
  input: Readonly<AuthScreenInput>,
): AuthScreenState {
  const isWaiting = input.authLoading || input.authUrl !== null;
  if (input.authInterrupted || (!input.isConnected && isWaiting))
    return 'interrupted';
  if (!input.isConnected) return 'engineDown';
  if (input.authTimedOut) return 'timedOut';
  if (isWaiting) return 'waiting';
  if (input.authError !== null) return 'failed';
  return 'idle';
}

export function isEngineConnected(status: string): boolean {
  return ['connected', 'ready', 'syncing', 'rate_limited'].includes(status);
}
