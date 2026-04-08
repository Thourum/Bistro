import React from 'react';
import { AuthLayout } from '../layouts/AuthLayout';
import { ProviderButton } from '../components/ProviderButton';
import { useWebSocket } from '../hooks/useWebSocket';
import { useStore } from '../hooks/useStore';
import { appStore } from '../store';
import type { UIState } from '../store/ui';

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

  const handleProviderClick = (provider: OAuthProvider): void => {
    // TODO: Trigger full OAuth flow via IPC — window.electronAPI.startOAuth(provider)
    // Notify sync engine that auth is starting
    send('AUTH_START', { provider });
    // Optimistically mark as authenticated (replace with real callback once IPC OAuth is wired)
    appStore.setState((s) => ({ ...s, isAuthenticated: true }));
  };

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

        {/* Provider buttons */}
        <div className="space-y-3">
          {PROVIDERS.map(({ provider, icon, label, keyHint }) => (
            <ProviderButton
              key={provider}
              provider={provider}
              icon={icon}
              label={label}
              keyHint={keyHint}
              onClick={() => handleProviderClick(provider)}
            />
          ))}
        </div>

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
