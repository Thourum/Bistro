import React, { useCallback } from 'react';
import { ProviderButton } from '../components/ProviderButton';
import { useStore } from '../hooks/useStore';
import { useWebSocket } from '../hooks/useWebSocket';
import { AuthLayout } from '../layouts/AuthLayout';
import type { ProviderId } from '../providers';
import { PROVIDERS, parseAuthAction } from '../providers';
import { appStore } from '../store';
import type { UIState } from '../store/ui';
import { createLogger } from '../utils/logger';

const log = createLogger('LoginPage');

export function LoginPage() {
  const { send } = useWebSocket();
  const syncStatus = useStore(appStore, (s: UIState) => s.syncStatus);
  const syncError = useStore(appStore, (s: UIState) => s.syncError);
  const lastSyncAt = useStore(appStore, (s: UIState) => s.lastSyncAt);
  const authLoading = useStore(appStore, (s: UIState) => s.authLoading);
  const authError = useStore(appStore, (s: UIState) => s.authError);

  const handleProviderClick = useCallback(
    (provider: ProviderId): void => {
      // Read fresh from store — the closure value may be stale on rapid re-invocation
      if (appStore.state.authLoading) {
        log.warn({ provider }, 'AUTH_START ignored — already loading');
        return;
      }

      // Set loading eagerly before sending to close the race window
      appStore.setState((s) => ({ ...s, authLoading: true, authError: null }));

      log.info({ provider, syncStatus }, 'provider clicked');

      const msgId = send('AUTH_START', { provider });

      if (msgId === undefined) {
        log.error(
          { provider, syncStatus },
          'send returned undefined — WebSocket not connected',
        );
        appStore.setState((s) => ({
          ...s,
          authLoading: false,
          authError: `Cannot connect to sync engine (status: ${syncStatus}). Make sure the Go process is running.`,
        }));
        return;
      }

      log.info({ provider, msgId }, 'AUTH_START sent');
    },
    [send, syncStatus],
  );

  // React to keyboard bindings dispatched via the centralized keyboard system.
  // The binding sets confirmingAction to 'auth_github' / 'auth_gitlab' / 'auth_gitea'.
  const confirmingAction = useStore(
    appStore,
    (s: UIState) => s.confirmingAction,
  );

  React.useEffect(() => {
    if (confirmingAction === null) return;

    const parsed = parseAuthAction(confirmingAction);
    appStore.setState((s) => ({ ...s, confirmingAction: null }));

    if (parsed === null) return;
    if (authLoading) return;

    handleProviderClick(parsed.id as ProviderId);
  }, [confirmingAction, authLoading, handleProviderClick]);

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

        {/* Auth state UI */}
        {authLoading ? (
          <div className="text-center space-y-4">
            <p className="text-sm text-on-surface-variant animate-pulse">
              Waiting for authorization...
            </p>
            <p className="text-xs text-on-surface-variant">
              Complete the sign-in in your browser, then return here.
            </p>
          </div>
        ) : (
          <>
            {/* Provider buttons */}
            <div className="space-y-3">
              {PROVIDERS.map((p) => (
                <ProviderButton
                  key={p.id}
                  provider={p.id}
                  icon={p.icon}
                  label={p.label}
                  keyHint={p.key.toUpperCase()}
                  onClick={() => {
                    if (!authLoading) {
                      handleProviderClick(p.id);
                    }
                  }}
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
