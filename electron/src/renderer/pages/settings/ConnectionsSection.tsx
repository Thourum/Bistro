import { Button } from '../../components/ui/button';
import { useStore } from '../../hooks/useStore';
import { PROVIDERS, type ProviderId } from '../../providers';
import { startSignIn } from '../../services/signIn';
import { wsClient } from '../../services/ws/connection';
import { appStore } from '../../store';
import type { UIState } from '../../store/ui';
import { Section } from './SettingsComponents';

export function ConnectionsSection() {
  const connected = useStore(appStore, (s: UIState) => s.connectedProviders);
  const authLoading = useStore(appStore, (s: UIState) => s.authLoading);

  const handleConnect = (providerId: ProviderId) => startSignIn(providerId);

  const handleDisconnect = (providerId: string) => {
    wsClient.send('AUTH_DISCONNECT', { provider: providerId });
  };

  return (
    <Section id="connections" title="Connections">
      <div className="grid grid-cols-1 gap-px bg-outline-variant/20 border border-outline-variant/20">
        {PROVIDERS.map((provider) => {
          const link = connected.find((c) => c.provider === provider.id);
          const isConnected = link !== undefined;
          return (
            <div
              key={provider.id}
              className="flex items-center justify-between p-6 md:p-8 bg-surface group hover:bg-surface-container-low transition-colors"
            >
              <div className="flex items-center gap-6">
                <div className="w-12 h-12 bg-slate-900 flex items-center justify-center">
                  <span className="material-symbols-outlined text-white text-[24px]">
                    {provider.icon}
                  </span>
                </div>
                <div>
                  <p className="font-bold text-base text-on-surface">
                    {provider.label}
                  </p>
                  <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
                    {isConnected
                      ? `Connected as @${link.username || 'unknown'}`
                      : 'Not connected'}
                  </p>
                </div>
              </div>
              {isConnected ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handleDisconnect(provider.id)}
                  className="px-5"
                >
                  Disconnect
                </Button>
              ) : (
                <Button
                  size="sm"
                  disabled={authLoading}
                  onClick={() => handleConnect(provider.id)}
                  className="px-5"
                >
                  Connect
                </Button>
              )}
            </div>
          );
        })}
      </div>
    </Section>
  );
}
