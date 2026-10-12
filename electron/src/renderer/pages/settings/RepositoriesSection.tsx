import { err, ok, type Result, ResultAsync } from 'neverthrow';
import { useCallback, useState } from 'react';
import { CacheUsageCard } from '../../components/CacheUsageCard';
import { addToast } from '../../components/ToastContainer';
import { Button } from '../../components/ui/button';
import { useStore } from '../../hooks/useStore';
import { useWebSocket } from '../../hooks/useWebSocket';
import { settingsStore, updateSettings } from '../../store/settings';
import type { LocationErrorCode } from '../../types/repoLocation';
import { Section } from './SettingsComponents';

interface ValidateDevFolderResponse {
  ok: true;
  canonical_path: string;
}

interface LocationErrorResponse {
  ok?: false;
  repo_id: string;
  operation: string;
  code: LocationErrorCode;
  message: string;
}

interface ScanDevFolderSummary {
  ok: true;
  discovered: number;
  healthy: number;
  degraded: number;
  skipped: number;
  partial: boolean;
}

const LOCATION_ERROR_CODES = [
  'path_missing',
  'remote_mismatch',
  'permission_denied',
  'invalid_url',
  'invalid_path',
  'destination_required',
  'destination_conflict',
  'clone_failed',
  'auth_failed',
  'cancelled',
  'delete_failed',
  'scan_partial',
] as const satisfies readonly LocationErrorCode[];

function isRecord(data: unknown): data is Record<string, unknown> {
  return typeof data === 'object' && data !== null;
}

function isLocationErrorCode(code: unknown): code is LocationErrorCode {
  return LOCATION_ERROR_CODES.includes(
    code as (typeof LOCATION_ERROR_CODES)[number],
  );
}

function parseValidateDevFolderResponse(
  data: unknown,
): Result<ValidateDevFolderResponse, string> {
  if (
    isRecord(data) &&
    data.ok === true &&
    typeof data.canonical_path === 'string'
  ) {
    return ok({ ok: true, canonical_path: data.canonical_path });
  }

  return err('Dev folder validation returned an invalid response.');
}

function parseScanDevFolderSummary(
  data: unknown,
): Result<ScanDevFolderSummary, string> {
  if (
    isRecord(data) &&
    data.ok === true &&
    typeof data.discovered === 'number' &&
    typeof data.healthy === 'number' &&
    typeof data.degraded === 'number' &&
    typeof data.skipped === 'number' &&
    typeof data.partial === 'boolean'
  ) {
    return ok({
      ok: true,
      discovered: data.discovered,
      healthy: data.healthy,
      degraded: data.degraded,
      skipped: data.skipped,
      partial: data.partial,
    });
  }

  return err('Scan failed.');
}

function isLocationErrorResponse(data: unknown): data is LocationErrorResponse {
  return (
    isRecord(data) &&
    data.ok !== true &&
    typeof data.repo_id === 'string' &&
    typeof data.operation === 'string' &&
    isLocationErrorCode(data.code) &&
    typeof data.message === 'string'
  );
}

function scanStatusText(scan: ScanDevFolderSummary | null): string {
  if (scan === null) {
    return 'No scan has run yet.';
  }

  const partial = scan.partial ? ' Partial scan; check the logs.' : '';
  return `${scan.discovered} discovered, ${scan.healthy} healthy, ${scan.degraded} degraded, ${scan.skipped} skipped.${partial}`;
}

function ensurePromise(value: Promise<unknown> | unknown): Promise<unknown> {
  return value instanceof Promise ? value : Promise.resolve(value);
}

function requestErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim() !== '') {
    return error.message;
  }

  if (typeof error === 'string' && error.trim() !== '') {
    return error;
  }

  return 'Bistro could not reach the sync engine. Try again.';
}

function pickFolderResult(): ResultAsync<string | null, string> {
  if (window.electron?.pickFolder === undefined) {
    return ResultAsync.fromSafePromise(
      Promise.resolve<string | null>(null),
    ).andThen(() => err('Folder picker is unavailable.'));
  }

  return ResultAsync.fromPromise(
    window.electron.pickFolder(),
    () => 'Dev folder could not be changed.',
  );
}

export function RepositoriesSection() {
  const repositories = useStore(settingsStore, (s) => s.repositories);
  const { request } = useWebSocket();
  const [scanResult, setScanResult] = useState<ScanDevFolderSummary | null>(
    null,
  );
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isChanging, setIsChanging] = useState(false);
  const currentPath = repositories.devFolderPath;

  const scanDevFolder = useCallback(
    (path: string): ResultAsync<ScanDevFolderSummary, string> =>
      ResultAsync.fromPromise(
        ensurePromise(request('SCAN_DEV_FOLDER', { path, reason: 'settings' })),
        requestErrorMessage,
      )
        .andThen(parseScanDevFolderSummary)
        .map((summary) => {
          setScanResult(summary);
          setErrorMessage(null);
          return summary;
        })
        .mapErr((message) => {
          setErrorMessage(message);
          return message;
        }),
    [request],
  );

  const handleChange = (): void => {
    setIsChanging(true);
    pickFolderResult()
      .andThen((path) => {
        if (path === null) {
          return ResultAsync.fromSafePromise(Promise.resolve(null));
        }

        return ResultAsync.fromPromise(
          ensurePromise(request('VALIDATE_DEV_FOLDER', { path })),
          requestErrorMessage,
        )
          .andThen((response) => {
            if (isLocationErrorResponse(response)) {
              return err(response.message);
            }

            return parseValidateDevFolderResponse(response);
          })
          .andThen((response: ValidateDevFolderResponse) =>
            updateSettings({
              repositories: {
                ...repositories,
                devFolderPath: response.canonical_path,
              },
            })
              .mapErr(() => 'Dev folder could not be changed.')
              .map(() => response.canonical_path),
          )
          .andThen(scanDevFolder)
          .map(() => null);
      })
      .match(
        () => setIsChanging(false),
        (message) => {
          setErrorMessage(message);
          addToast({ type: 'error', message, dismissible: true });
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
      repositories: { ...repositories, devFolderPath: null },
    })
      .mapErr(() => 'Dev folder could not be cleared.')
      .match(
        () => {
          setScanResult(null);
          setErrorMessage(null);
          setIsChanging(false);
        },
        (message) => {
          setErrorMessage(message);
          addToast({ type: 'error', message, dismissible: true });
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
