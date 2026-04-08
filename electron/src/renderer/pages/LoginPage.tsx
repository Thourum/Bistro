import React from 'react';
import { AuthLayout } from '../layouts/AuthLayout';
import { ProviderButton } from '../components/ProviderButton';

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

function handleProviderClick(provider: OAuthProvider): void {
  // TODO: Trigger OAuth flow via IPC — window.electronAPI.startOAuth(provider)
  // Once OAuth completes, set isAuthenticated: true in the store
  void provider;
}

export function LoginPage() {
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
            <span className="uppercase tracking-widest mb-1">Status: Ready</span>
            <span>v4.0.2-stable</span>
          </div>
          <div className="w-24 h-[1px] bg-outline-variant/30"></div>
        </div>
      </div>
    </AuthLayout>
  );
}
