import {
  DevFolderScan,
  type ScanDevFolderResult,
  Shell,
} from '@bindings/services';
import { Fault, okAsync, type ResultAsync } from '@thourum/fault';
import { useCallback, useState } from 'react';
import { CacheUsageCard } from '../../components/CacheUsageCard';
import { addToast } from '../../components/ToastContainer';
import { Button } from '../../components/ui/button';
import { useStore } from '../../hooks/useStore';
import { call } from '../../services/engine/call';
import { settingsStore, updateSettings } from '../../store/settings';
import { Section } from './SettingsComponents';

type ScanDevFolderSummary = ScanDevFolderResult;

function scanStatusText(scan: ScanDevFolderSummary | null): string {
  if (scan === null) {
    return 'No scan has run yet.';
  }

  const partial = scan.partial ? ' Partial scan; check the logs.' : '';
  return `${scan.discovered} discovered, ${scan.healthy} healthy, ${scan.degraded} degraded, ${scan.skipped} skipped.${partial}`;
}

export function RepositoriesSection() {
  const repositories = useStore(settingsStore, (s) => s.repositories);
  const [scanResult, setScanResult] = useState<ScanDevFolderSummary | null>(
    null,
  );
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isChanging, setIsChanging] = useState(false);
  const currentPath = repositories.devFolderPath;

  const scanDevFolder = useCallback(
    (path: string): ResultAsync<ScanDevFolderSummary, Fault> =>
      call(() => DevFolderScan.Scan({ path, reason: 'settings' }), {
        timeoutMs: null,
      })
        .map((summary) => {
          setScanResult(summary);
          setErrorMessage(null);
          return summary;
        })
        .mapErr((error) => {
          setErrorMessage(error.message);
          return error;
        }),
    [],
  );

  const handleChange = (): void => {
    setIsChanging(true);
    call(() => Shell.PickFolder())
      .map((path) => (path === '' ? null : path))
      .andThen((path) => {
        if (path === null) {
          return okAsync(null);
        }

        return call(() => DevFolderScan.Validate({ path }))
          .andThen((response) =>
            updateSettings({
              repositories: {
                devFolderPath: response.canonical_path,
              },
            }).map(() => response.canonical_path),
          )
          .andThen(scanDevFolder)
          .map(() => null);
      })
      .match(
        () => setIsChanging(false),
        (error) => {
          setErrorMessage(error.message);
          addToast({
            type: 'error',
            message: error.message,
            dismissible: true,
          });
          setIsChanging(false);
        },
      );
  };

  const handleClear = (): void => {
    if (isChanging) {
      return;
    }
    setIsChanging(true);
    updateSettings({
      repositories: { devFolderPath: null },
    })
      .mapErr((cause) =>
        new Fault('Dev folder could not be cleared.')
          .withTag('SETTINGS_ERROR')
          .withCause(cause),
      )
      .match(
        () => {
          setScanResult(null);
          setErrorMessage(null);
          setIsChanging(false);
        },
        (error) => {
          setErrorMessage(error.message);
          addToast({
            type: 'error',
            message: error.message,
            dismissible: true,
          });
          setIsChanging(false);
        },
      );
  };

  return (
    <Section id="repositories" title="Repositories">
      <div className="space-y-6">
        <div className="border border-outline-variant/20 bg-surface p-6 md:p-8 space-y-4">
          <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
            <div>
              <p className="font-bold text-sm text-on-surface">Dev folder</p>
              <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
                {currentPath ?? 'Not set'}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                size="sm"
                disabled={isChanging}
                onClick={handleChange}
              >
                Change
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={currentPath === null || isChanging}
                onClick={handleClear}
              >
                Clear
              </Button>
            </div>
          </div>
          <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
            {scanStatusText(scanResult)}
          </p>
          {errorMessage !== null && (
            <p role="alert" className="text-sm text-error">
              {errorMessage}
            </p>
          )}
        </div>
        <CacheUsageCard />
      </div>
    </Section>
  );
}
