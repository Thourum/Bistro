import { Fault, fromPromise } from '@thourum/fault';
import { useState } from 'react';
import type { ClassificationSettings } from '../../../types/settings';
import { addToast } from '../../components/ToastContainer';
import { Button } from '../../components/ui/button';
import { Input } from '../../components/ui/input';
import { useStore } from '../../hooks/useStore';
import { useWebSocket } from '../../hooks/useWebSocket';
import { appStore } from '../../store';
import type { UIState } from '../../store/ui';
import { saveSettings } from './SettingsComponents';

interface ClassificationProviderProps {
  classification: ClassificationSettings;
  isAIMode: boolean;
}

const PROVIDER_OPTIONS = [
  { id: 'anthropic', label: 'Anthropic' },
  { id: 'openai', label: 'OpenAI' },
  { id: 'openrouter', label: 'OpenRouter' },
  { id: 'ollama', label: 'Ollama' },
  { id: 'custom', label: 'Custom (OpenAI-compatible)' },
] as const satisfies ReadonlyArray<{
  id: ClassificationSettings['provider'];
  label: string;
}>;

export function ClassificationProvider({
  classification,
  isAIMode,
}: ClassificationProviderProps) {
  const byokSessionTokens = useStore(
    appStore,
    (s: UIState) => s.byokSessionTokens,
  );
  const byokEstCost = useStore(appStore, (s: UIState) => s.byokEstCost);
  const byokAuthFailed = useStore(appStore, (s: UIState) => s.byokAuthFailed);
  const { request } = useWebSocket();
  const [keyInput, setKeyInput] = useState('');
  const [keySaved, setKeySaved] = useState(false);
  const [keySaving, setKeySaving] = useState(false);

  const saveKey = (): void => {
    if (keyInput.length === 0 || keySaving) return;
    setKeySaving(true);
    // The key travels ONLY through SET_BYOK_KEY into the OS keychain —
    // never into settings JSON or any renderer store.
    void fromPromise(
      request('SET_BYOK_KEY', {
        provider: classification.provider,
        key: keyInput,
      }),
      (cause) =>
        new Fault(cause instanceof Error ? cause : 'sync engine unreachable')
          .withTag('CONNECTION_ERROR')
          .withCause(cause),
    ).match(
      () => {
        setKeySaving(false);
        setKeyInput('');
        setKeySaved(true);
        appStore.setState((s) => ({ ...s, byokAuthFailed: false }));
        addToast({
          type: 'success',
          message: `API key saved to the OS keychain (${classification.provider})`,
          dismissible: true,
        });
      },
      (err) => {
        // Key kept in the input so the user can retry.
        setKeySaving(false);
        addToast({
          type: 'error',
          message: `Saving key failed: ${err.message}`,
          dismissible: true,
        });
      },
    );
  };

  return (
    <div
      className={
        isAIMode ? 'space-y-3 border border-outline-variant/20 p-4' : 'hidden'
      }
    >
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <label className="block" htmlFor="byok-provider">
          <span className="block text-xs text-on-surface-variant mb-1">
            Provider
          </span>
          <select
            id="byok-provider"
            value={classification.provider}
            onChange={(event) => {
              const provider = PROVIDER_OPTIONS.find(
                (option) => option.id === event.target.value,
              );
              if (provider !== undefined) {
                saveSettings({
                  classification: { ...classification, provider: provider.id },
                });
              }
            }}
            className="w-full bg-surface-container-lowest border border-outline-variant/40 px-3 py-2 text-sm text-on-surface"
          >
            {PROVIDER_OPTIONS.map((option) => (
              <option key={option.id} value={option.id}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <label className="block" htmlFor="byok-model">
          <span className="block text-xs text-on-surface-variant mb-1">
            Model
          </span>
          <Input
            id="byok-model"
            value={classification.model}
            placeholder="e.g. claude-haiku-4-5"
            onChange={(e) =>
              saveSettings({
                classification: {
                  ...classification,
                  model: e.target.value,
                },
              })
            }
          />
        </label>
      </div>
      {(classification.provider === 'custom' ||
        classification.provider === 'ollama') && (
        <label className="block" htmlFor="byok-endpoint">
          <span className="block text-xs text-on-surface-variant mb-1">
            Endpoint
          </span>
          <Input
            id="byok-endpoint"
            value={classification.endpoint}
            placeholder="http://localhost:11434/v1"
            onChange={(e) =>
              saveSettings({
                classification: {
                  ...classification,
                  endpoint: e.target.value,
                },
              })
            }
          />
        </label>
      )}
      <div className="flex items-end gap-3">
        <label className="block flex-1" htmlFor="byok-key">
          <span
            className={`block text-xs mb-1 ${
              byokAuthFailed ? 'text-error' : 'text-on-surface-variant'
            }`}
          >
            API key{' '}
            {byokAuthFailed
              ? '— rejected by provider'
              : '(stored in the OS keychain, never in settings)'}
          </span>
          <Input
            id="byok-key"
            type="password"
            value={keyInput}
            placeholder={keySaved ? '••••••••  (key saved)' : 'sk-…'}
            onChange={(e) => setKeyInput(e.target.value)}
            className={byokAuthFailed ? 'border-error' : ''}
          />
        </label>
        <Button
          onClick={saveKey}
          disabled={keyInput.length === 0 || keySaving}
          className="px-5 py-2"
        >
          {keySaving ? 'Saving…' : 'Save key'}
        </Button>
      </div>
      <p className="font-mono text-[10px] text-outline">
        This session: {byokSessionTokens.toLocaleString()} tokens, est. $
        {byokEstCost.toFixed(2)}
      </p>
    </div>
  );
}
