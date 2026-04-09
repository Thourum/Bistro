import React from 'react';
import { AuthLayout } from '../layouts/AuthLayout';
import { ProviderButton } from '../components/ProviderButton';
import { useWebSocket } from '../hooks/useWebSocket';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { createLogger } from '../utils/logger';

const log = createLogger('LoginPage');

type OAuthProvider = 'github' | 'gitlab' | 'gitea';

const PROVIDERS: Array<{
  provider: OAuthProvider;
  icon: string;
  label: string;
  keyHint: string;
}> = [
  { provider: 'github', icon: 'terminal', label: 'GitHub', keyHint: 'G' },
  { provider: 'gitlab', icon: 'source', label: 'GitLab', keyHint: 'L' },
  { provider: 'gitea', icon: 'coffee', label: 'Gitea', keyHint: 'T' },
];

export function LoginPage() {
  const { send } = useWebSocket();
  const syncStatus = useStore(appStore, (s: UIState) => s.syncStatus);
  const syncError = useStore(appStore, (s: UIState) => s.syncError);
  const lastSyncAt = useStore(appStore, (s: UIState) => s.lastSyncAt);
  const deviceCode = useStore(appStore, (s: UIState) => s.deviceCode);
  const verificationUri = useStore(appStore, (s: UIState) => s.verificationUri);
  const authLoading = useStore(appStore, (s: UIState) => s.authLoading);
  const authError = useStore(appStore, (s: UIState) => s.authError);

  const handleProviderClick = (provider: OAuthProvider): void => {
    log.info({ provider, syncStatus, authLoading }, 'provider clicked');

    const msgId = send('AUTH_START', { provider });

    if (msgId === undefined) {
      log.error({ provider, syncStatus }, 'send returned undefined — WebSocket not connected');
      appStore.setState((s) => ({
        ...s,
        authError: `Cannot connect to sync engine (status: ${syncStatus}). Make sure the Go process is running.`,
      }));
    } else {
      log.info({ provider, msgId }, 'AUTH_START sent');
      appStore.setState((s) => ({ ...s, authLoading: true, authError: null }));
    }
  };

  // React to keyboard bindings dispatched via the centralized keyboard system.
  // The binding sets confirmingAction to 'auth_github' / 'auth_gitlab' / 'auth_gitea'.
  const confirmingAction = useStore(appStore, (s: UIState) => s.confirmingAction);

  React.useEffect(() => {
    if (confirmingAction === null) return;

    const match = confirmingAction.match(/^auth_(github|gitlab|gitea)$/);
    if (match === null) return;

    if (authLoading || deviceCode !== null) {
      appStore.setState((s) => ({ ...s, confirmingAction: null }));
      return;
    }

    const provider = match[1] as OAuthProvider;
    appStore.setState((s) => ({ ...s, confirmingAction: null }));
    handleProviderClick(provider);
  }, [confirmingAction, authLoading, deviceCode]);

  return (
    <AuthLayout>
      <div className="p-12">
        {/* Header */}
        <header className="mb-12">
          <h1 className="text-4xl font-extrabold tracking-tighter text-on-surface mb-4 uppercase">
            BISTRO
          </h1>
          <p className="text-on-surface-variant text-sm font-medium tracking-tight">
            Connect your platform to begin reviewing code
          </p>
        </header>

        {/* Device code flow UI */}
        {deviceCode !== null && verificationUri !== null ? (
          <div className="text-center space-y-4">
            <p className="text-sm text-on-surface-variant">
              Enter this code at GitHub:
            </p>
            <p className="text-3xl font-mono font-bold text-primary tracking-widest">
              {deviceCode}
            </p>
            <a
              href={verificationUri}
              target="_blank"
              rel="noopener noreferrer"
              className="text-sm text-primary hover:underline"
            >
              Open {verificationUri}
            </a>
            <p className="text-xs text-on-surface-variant animate-pulse">
              Waiting for authorization...
            </p>
          </div>
        ) : (
          <>
            {/* Provider buttons */}
            <div className="space-y-3">
              {PROVIDERS.map(({ provider, icon, label, keyHint }) => (
                <ProviderButton
                  key={provider}
                  provider={provider}
                  icon={icon}
                  label={label}
                  keyHint={keyHint}
                  onClick={() => { if (!authLoading) { handleProviderClick(provider); } }}
                />
              ))}
            </div>

            {authError !== null && (
              <p className="mt-4 text-sm text-error">{authError}</p>
            )}
          </>
        )}

        {/* Footer */}
        <div className="mt-16 flex items-end justify-between">
          <div className="font-mono text-[10px] text-outline-variant flex flex-col">
            <span className="uppercase tracking-widest mb-1">
              Status: {syncStatus.toUpperCase()}
            </span>
            {syncError !== null && (
              <span className="text-error text-[9px]">{syncError}</span>
            )}
            {lastSyncAt !== null && (
              <span className="text-[9px]">
                Last sync: {new Date(lastSyncAt).toLocaleTimeString()}
              </span>
            )}
            <span>v4.0.2-stable</span>
          </div>
          <div className="w-24 h-[1px] bg-outline-variant/30"></div>
        </div>
      </div>
    </AuthLayout>
  );
}
