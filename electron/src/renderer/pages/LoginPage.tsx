import type React from 'react';
import { ProviderButton } from '../components/ProviderButton';
import { useEffectiveBindings } from '../hooks/useEffectiveBindings';
import { useSignInScreenData } from '../hooks/useSignInScreenData';
import { AuthLayout } from '../layouts/AuthLayout';
import { PROVIDERS } from '../providers';
import { startSignIn } from '../services/signIn';
import { findActionKeys } from '../utils/legendPresets';
import { SignInState } from './login/SignInState';
import { TrustPanel } from './login/TrustPanel';

export function LoginPage(): React.ReactElement {
  const {
    screen,
    elapsed,
    isConnected,
    authLoading,
    authError,
    authExchange,
    engineStatus,
    appVersion,
    lastSyncAt,
    syncError,
    platform,
  } = useSignInScreenData();
  const githubHint = findActionKeys(useEffectiveBindings(), [
    'login.provider.github',
  ]);
  const isShowingHosts = screen !== 'waiting' && screen !== 'timedOut';

  // AuthLayout grows with flex-1; RootShell's route slot is not a flex box.
  return (
    <div className="flex h-full">
      <AuthLayout isWide>
        <div className="grid md:grid-cols-2">
          <div className="flex flex-col gap-8 p-8 md:p-10">
            <header className="space-y-3">
              <p className="text-4xl font-extrabold uppercase tracking-tighter text-on-background">
                BISTRO
              </p>
              <h1 className="text-sm font-medium tracking-tight text-on-surface-variant">
                Connect your platform to begin reviewing code
              </h1>
            </header>
            <div className="space-y-4">
              {isShowingHosts && (
                <div className="space-y-3">
                  {PROVIDERS.map((provider) => (
                    <ProviderButton
                      key={provider.id}
                      provider={provider.id}
                      icon={provider.icon}
                      label={provider.label}
                      disabled={
                        !provider.isAvailable || !isConnected || authLoading
                      }
                      autoFocus={provider.isAvailable}
                      keyHint={provider.isAvailable ? githubHint : undefined}
                      availabilityLabel={
                        provider.isAvailable ? undefined : 'Coming soon'
                      }
                      onClick={() => startSignIn(provider.id)}
                    />
                  ))}
                </div>
              )}
              <SignInState
                screen={screen}
                elapsed={elapsed}
                error={authError}
                isConnected={isConnected}
                status={engineStatus}
                syncError={syncError}
                lastSyncAt={lastSyncAt}
                version={appVersion}
              />
            </div>
            <p className="mt-auto font-mono text-[11px] text-outline">
              Sign-in opens GitHub in your browser. Bistro never sees your
              password.
            </p>
          </div>
          <TrustPanel
            exchange={authExchange}
            isConnected={isConnected}
            platform={platform}
          />
        </div>
      </AuthLayout>
    </div>
  );
}
