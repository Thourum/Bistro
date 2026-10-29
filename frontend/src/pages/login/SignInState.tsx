import type { EngineStatus } from '@renderer/types/electron/engine';
import type React from 'react';
import { ActionKeyHint } from '../../components/ui/action-key-hint';
import { Button } from '../../components/ui/button';
import {
  cancelSignIn,
  copySignInLink,
  reopenSignIn,
  retrySignIn,
} from '../../services/signIn';
import type { AuthScreenState } from '../../utils/authScreenState';

interface SignInAction {
  readonly actionId: string;
  readonly label: string;
  readonly run: () => void;
}

const WAITING_ACTIONS: readonly SignInAction[] = [
  { actionId: 'login.reopen', label: 'Reopen browser', run: reopenSignIn },
  {
    actionId: 'login.copyLink',
    label: 'Copy sign-in link',
    run: copySignInLink,
  },
  { actionId: 'login.cancel', label: 'Cancel', run: cancelSignIn },
];
const TIMEOUT_ACTIONS: readonly SignInAction[] = [
  { actionId: 'login.retry', label: 'Try again', run: retrySignIn },
  { actionId: 'login.cancel', label: 'Back to hosts', run: cancelSignIn },
];
const ACTIONS_BY_SCREEN: Partial<
  Record<AuthScreenState, readonly SignInAction[]>
> = { waiting: WAITING_ACTIONS, timedOut: TIMEOUT_ACTIONS };

const STATUS_ROW = 'grid grid-cols-[6rem_1fr] gap-4';

interface SignInStateProps {
  readonly screen: AuthScreenState;
  readonly elapsed: string;
  readonly error: string | null;
  readonly isConnected: boolean;
  readonly status: EngineStatus | null;
  readonly syncError: string | null;
  readonly lastSyncAt: string | null;
  readonly version: string | null;
}

function ActionRow({
  actions,
}: {
  readonly actions: readonly SignInAction[];
}): React.ReactElement {
  return (
    <div className="flex flex-wrap gap-2 border-t border-outline-variant/40 p-4">
      {actions.map((action, index) => (
        <Button
          key={action.actionId}
          variant={index === 0 ? 'outline' : 'ghost'}
          size="sm"
          onClick={action.run}
        >
          {action.label}
          <ActionKeyHint actionId={action.actionId} />
        </Button>
      ))}
    </div>
  );
}

function EngineDiagnostics(
  props: Pick<
    SignInStateProps,
    'status' | 'syncError' | 'lastSyncAt' | 'version'
  >,
): React.ReactElement {
  return (
    <div role="alert" className="space-y-4">
      <p className="text-sm text-error">
        {props.status?.isSpawned
          ? 'Cannot connect to sync engine. Bistro is restarting it.'
          : 'Cannot connect to sync engine. Start it, then Bistro reconnects automatically.'}
      </p>
      <dl className="space-y-1 border-t border-outline-variant/40 pt-4 font-mono text-[11px]">
        <div className={STATUS_ROW}>
          <dt className="uppercase tracking-widest text-outline">Engine</dt>
          <dd className="uppercase text-error">
            {props.status?.state ?? 'Unavailable'}
          </dd>
        </div>
        <div className={STATUS_ROW}>
          <dt className="uppercase tracking-widest text-outline">Error</dt>
          <dd className="break-words text-error">
            {props.syncError ??
              props.status?.error ??
              'No error detail available.'}
          </dd>
        </div>
        <div className={STATUS_ROW}>
          <dt className="uppercase tracking-widest text-outline">Last sync</dt>
          <dd className="text-on-surface-variant">
            {props.lastSyncAt === null
              ? 'Not yet synced'
              : new Date(props.lastSyncAt).toLocaleString()}
          </dd>
        </div>
        <div className={STATUS_ROW}>
          <dt className="uppercase tracking-widest text-outline">Version</dt>
          <dd className="text-on-surface-variant">
            {props.version ?? 'Unavailable'}
          </dd>
        </div>
      </dl>
    </div>
  );
}

/** Left-column state area: waiting/timeout panels, inline errors, engine diagnostics. */
export function SignInState(props: SignInStateProps): React.ReactElement {
  const actions = ACTIONS_BY_SCREEN[props.screen];
  return (
    <div className="space-y-4">
      {props.screen === 'waiting' && (
        <div className="border border-outline-variant">
          <div role="status" className="space-y-2 bg-surface-container-low p-4">
            <p className="flex flex-wrap items-baseline justify-between gap-2 font-mono text-xs uppercase tracking-widest">
              <span className="flex items-center gap-2 text-on-surface">
                <span
                  aria-hidden
                  className="h-2 w-2 bg-tertiary motion-safe:animate-pulse"
                />
                Waiting for authorization…
              </span>
              <span
                aria-live="off"
                className="text-outline"
              >{`GITHUB // ${props.elapsed}`}</span>
            </p>
            <p className="text-sm text-on-surface-variant">
              Complete the sign-in in your browser, then return here.
            </p>
          </div>
          {actions !== undefined && <ActionRow actions={actions} />}
        </div>
      )}
      {props.screen === 'timedOut' && (
        <div className="border border-outline-variant">
          <div role="status" className="space-y-2 bg-surface-container-low p-4">
            <h2 className="font-bold text-on-surface">
              No response after 5:00
            </h2>
            <p className="text-sm text-on-surface-variant">
              The browser hasn't reported back, and Bistro stopped waiting.
              Start again to get a fresh sign-in.
            </p>
          </div>
          {actions !== undefined && <ActionRow actions={actions} />}
        </div>
      )}
      {props.screen === 'failed' && props.error !== null && (
        <p role="alert" className="text-sm text-error">
          {props.error}
        </p>
      )}
      {props.screen === 'interrupted' && (
        <p role="alert" className="text-sm text-error">
          Sign-in was interrupted because the connection to the sync engine
          dropped. Start again.
        </p>
      )}
      {!props.isConnected && (
        <EngineDiagnostics
          status={props.status}
          syncError={props.syncError}
          lastSyncAt={props.lastSyncAt}
          version={props.version}
        />
      )}
    </div>
  );
}
