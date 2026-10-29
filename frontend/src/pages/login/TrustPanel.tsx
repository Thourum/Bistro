import type React from 'react';
import { addToast } from '../../components/ToastContainer';
import { openUrlInBrowser } from '../../utils/openUrl';

const SOURCE_URL = 'https://github.com/Thourum/Bistro';

const EXCHANGE_COPY = {
  proxy:
    "Bistro's sign-in endpoint swaps it for a token and passes it straight back. It stores nothing.",
  local: 'Bistro swaps it for a token on this machine.',
} satisfies Record<'proxy' | 'local', string>;

const KEYRING_COPY: Readonly<Record<string, string>> = {
  darwin: 'macOS Keychain',
  win32: 'Windows Credential Manager',
  linux: 'the Secret Service keyring on Linux',
};

const SECTION_LABEL =
  'font-mono text-[10px] uppercase tracking-widest text-outline';
const FACT_ROW = 'grid grid-cols-[7rem_1fr] gap-4';

interface TrustPanelProps {
  readonly exchange: 'proxy' | 'local' | null;
  readonly isConnected: boolean;
  readonly platform: string;
}

function openSource(): void {
  void openUrlInBrowser(SOURCE_URL).match(
    () => undefined,
    (fault) =>
      addToast({ type: 'error', message: fault.message, dismissible: true }),
  );
}

/** Scopes, token path and defaults, shown in every sign-in state. */
export function TrustPanel({
  exchange,
  isConnected,
  platform,
}: TrustPanelProps): React.ReactElement {
  // Disconnected means the exchange mode is unknown; never guess it.
  const exchangeStep =
    isConnected && exchange !== null
      ? EXCHANGE_COPY[exchange]
      : 'Bistro swaps it for a token.';
  const keyring = KEYRING_COPY[platform] ?? "your operating system's keyring";

  return (
    <section className="space-y-8 border-t border-outline-variant/40 bg-surface-container-low p-8 text-sm text-on-surface-variant md:border-t-0 md:border-l">
      <div>
        <h2 className="text-xl font-bold tracking-tight text-on-surface">
          What GitHub will ask you to approve
        </h2>
        <dl className="mt-4 divide-y divide-outline-variant/40 border-y border-outline-variant/40">
          <div className={`${FACT_ROW} py-3`}>
            <dt className="font-mono font-bold text-on-surface">repo</dt>
            <dd>
              GitHub labels this 'full control of private repositories'. Bistro
              needs it to read the private PRs you review and to post your
              reviews.
            </dd>
          </div>
          <div className={`${FACT_ROW} py-3`}>
            <dt className="font-mono font-bold text-on-surface">read:user</dt>
            <dd>
              Your username, so Bistro knows which review requests are yours.
            </dd>
          </div>
        </dl>
      </div>
      <div>
        <h2 className={SECTION_LABEL}>Where the token goes</h2>
        {/* A real sequence: the order of the OAuth exchange. */}
        <ol className="mt-3 list-decimal space-y-2 pl-5 marker:font-mono marker:text-outline">
          <li>GitHub returns a one-time authorization code.</li>
          <li>{exchangeStep}</li>
          <li>Bistro saves the token in {keyring}.</li>
        </ol>
      </div>
      <dl className="space-y-2 border-t border-outline-variant/40 pt-6 font-mono text-xs">
        <div className={FACT_ROW}>
          <dt className="uppercase tracking-widest text-outline">Telemetry</dt>
          <dd className="text-on-surface">Off by default.</dd>
        </div>
        <div className={FACT_ROW}>
          <dt className="uppercase tracking-widest text-outline">
            Auto-update
          </dt>
          <dd className="text-on-surface">Off by default.</dd>
        </div>
        <div className={FACT_ROW}>
          <dt className="uppercase tracking-widest text-outline">Licence</dt>
          <dd className="text-on-surface">
            GPLv3 ·{' '}
            <button
              type="button"
              className="text-primary underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              onClick={openSource}
            >
              read the source
            </button>
          </dd>
        </div>
        <div className={FACT_ROW}>
          <dt className="uppercase tracking-widest text-outline">Revoke</dt>
          <dd className="text-on-surface">GitHub → Settings → Applications</dd>
        </div>
      </dl>
    </section>
  );
}
