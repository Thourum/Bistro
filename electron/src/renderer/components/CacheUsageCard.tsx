import { err, Fault, ok, type Result, ResultAsync } from '@thourum/fault';
import { useCallback, useEffect, useState } from 'react';
import { useWebSocket } from '../hooks/useWebSocket';
import type { LocationErrorCode } from '../types/repoLocation';
import { addToast } from './ToastContainer';

interface CacheRepoStat {
  repo_id: string;
  size_bytes: number;
  last_used_at: string | null;
  stale: boolean;
}

interface CacheStats {
  ok: true;
  total_bytes: number;
  active_bytes: number;
  stale_bytes: number;
  repos: CacheRepoStat[];
}

interface CacheFailure {
  repo_id: string;
  code: LocationErrorCode;
  message: string;
}

interface CacheMutationResponse {
  ok: true;
  removed?: string[];
  failed?: CacheFailure[];
  orphans_removed?: number;
}

interface LocationErrorResponse {
  ok?: false;
  repo_id: string;
  operation: string;
  code: LocationErrorCode;
  message: string;
}

type CacheActionResponse = CacheMutationResponse | LocationErrorResponse;

const DELETE_FAILED_MESSAGE = 'Something went wrong, check the logs';
const CLEAR_CACHE_CONFIRM_MESSAGE = 'Clear all cached repos?';

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

function parseRepoStat(data: unknown): CacheRepoStat | null {
  if (!isRecord(data)) {
    return null;
  }

  if (
    typeof data.repo_id === 'string' &&
    typeof data.size_bytes === 'number' &&
    (data.last_used_at === null || typeof data.last_used_at === 'string') &&
    typeof data.stale === 'boolean'
  ) {
    return {
      repo_id: data.repo_id,
      size_bytes: data.size_bytes,
      last_used_at: data.last_used_at,
      stale: data.stale,
    };
  }

  return null;
}

function parseCacheStats(data: unknown): Result<CacheStats, Fault> {
  if (!isRecord(data) || data.ok !== true || !Array.isArray(data.repos)) {
    return err(
      new Fault('Cache stats returned an invalid response.').withTag(
        'VALIDATION_ERROR',
      ),
    );
  }

  const repos = data.repos.map(parseRepoStat);
  if (
    typeof data.total_bytes !== 'number' ||
    typeof data.active_bytes !== 'number' ||
    typeof data.stale_bytes !== 'number' ||
    repos.some((repo) => repo === null)
  ) {
    return err(
      new Fault('Cache stats returned an invalid response.').withTag(
        'VALIDATION_ERROR',
      ),
    );
  }

  return ok({
    ok: true,
    total_bytes: data.total_bytes,
    active_bytes: data.active_bytes,
    stale_bytes: data.stale_bytes,
    repos: repos.filter((repo): repo is CacheRepoStat => repo !== null),
  });
}

function parseCacheFailure(data: unknown): CacheFailure | null {
  if (!isRecord(data)) {
    return null;
  }

  if (
    typeof data.repo_id === 'string' &&
    isLocationErrorCode(data.code) &&
    typeof data.message === 'string'
  ) {
    return {
      repo_id: data.repo_id,
      code: data.code,
      message: data.message,
    };
  }

  return null;
}

function parseCacheActionResponse(
  data: unknown,
): Result<CacheActionResponse, Fault> {
  if (!isRecord(data)) {
    return err(
      new Fault('Cache action returned an invalid response.').withTag(
        'VALIDATION_ERROR',
      ),
    );
  }

  if (data.ok === true) {
    const failed = Array.isArray(data.failed)
      ? data.failed.map(parseCacheFailure)
      : [];

    if (failed.some((failure) => failure === null)) {
      return err(
        new Fault('Cache action returned an invalid response.').withTag(
          'VALIDATION_ERROR',
        ),
      );
    }

    const validFailed = failed.filter(
      (failure): failure is CacheFailure => failure !== null,
    );
    const response: CacheMutationResponse = { ok: true, failed: validFailed };

    if (Array.isArray(data.removed)) {
      return ok({
        ...response,
        removed: data.removed.filter(
          (repoID): repoID is string => typeof repoID === 'string',
        ),
      });
    }

    if (typeof data.orphans_removed === 'number') {
      return ok({ ...response, orphans_removed: data.orphans_removed });
    }

    return ok(response);
  }

  if (
    isLocationErrorCode(data.code) &&
    typeof data.repo_id === 'string' &&
    typeof data.operation === 'string' &&
    typeof data.message === 'string'
  ) {
    return ok({
      ok: false,
      repo_id: data.repo_id,
      operation: data.operation,
      code: data.code,
      message: data.message,
    });
  }

  return err(
    new Fault('Cache action returned an invalid response.').withTag(
      'VALIDATION_ERROR',
    ),
  );
}

function ensurePromise(value: Promise<unknown> | unknown): Promise<unknown> {
  return value instanceof Promise ? value : Promise.resolve(value);
}

function requestFault(error: unknown): Fault {
  const message =
    error instanceof Error && error.message.trim() !== ''
      ? error.message
      : typeof error === 'string' && error.trim() !== ''
        ? error
        : 'Bistro could not reach the sync engine. Try again.';
  return new Fault(message).withTag('CONNECTION_ERROR').withCause(error);
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) {
    return `${bytes} B`;
  }

  const units = ['KB', 'MB', 'GB', 'TB'] as const;
  let value = bytes / 1024;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[unitIndex]}`;
}

function segmentWidth(bytes: number, total: number): string {
  if (total <= 0 || bytes <= 0) {
    return '0%';
  }

  return `${(bytes / total) * 100}%`;
}

function repoIdentity(repoID: string): string {
  const parts = repoID.split(':');
  return parts.length === 2 ? (parts[1] ?? repoID) : repoID;
}

function formatLastUsed(value: string | null): string {
  if (value === null || value.trim() === '') {
    return 'Never';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return 'Unknown';
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
  }).format(date);
}

export function CacheUsageCard(): React.ReactElement {
  const { request } = useWebSocket();
  const [stats, setStats] = useState<CacheStats | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isExpanded, setIsExpanded] = useState(false);
  const [pendingAction, setPendingAction] = useState<string | null>(null);

  const loadStats = useCallback((): void => {
    ResultAsync.fromPromise(
      ensurePromise(request('GET_CACHE_STATS', {})),
      requestFault,
    )
      .andThen(parseCacheStats)
      .match(
        (nextStats) => {
          setStats(nextStats);
          setErrorMessage(null);
        },
        (error) => setErrorMessage(error.message),
      );
  }, [request]);

  useEffect(() => {
    loadStats();
  }, [loadStats]);

  const runAction = (
    type: string,
    payload: unknown,
    actionKey: string,
  ): void => {
    setPendingAction(actionKey);
    ResultAsync.fromPromise(ensurePromise(request(type, payload)), requestFault)
      .andThen(parseCacheActionResponse)
      .match(
        (response) => {
          setPendingAction(null);
          if (
            response.ok !== true ||
            response.failed?.some(
              (failure) => failure.code === 'delete_failed',
            ) === true
          ) {
            addToast({
              type: 'error',
              message: DELETE_FAILED_MESSAGE,
              dismissible: true,
            });
            return;
          }
          loadStats();
        },
        () => {
          setPendingAction(null);
          addToast({
            type: 'error',
            message: DELETE_FAILED_MESSAGE,
            dismissible: true,
          });
        },
      );
  };

  const totalBytes = stats?.total_bytes ?? 0;
  const activeBytes = stats?.active_bytes ?? 0;
  const staleBytes = stats?.stale_bytes ?? 0;
  const repos = stats?.repos ?? [];

  return (
    <div className="border border-outline-variant/20 bg-surface p-6 md:p-8 space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 className="font-bold text-base text-on-surface">Cache usage</h3>
          <p className="font-mono text-[11px] text-on-surface-variant uppercase tracking-tight">
            Bistro-managed repositories only
          </p>
        </div>
        <p className="font-mono text-sm font-bold text-on-surface text-right">
          {formatBytes(totalBytes)}
        </p>
      </div>

      <div className="flex h-3 w-full overflow-hidden bg-surface-container-highest">
        <div
          data-testid="cache-active-segment"
          className="h-full bg-primary"
          style={{ width: segmentWidth(activeBytes, totalBytes) }}
        />
        <div
          data-testid="cache-stale-segment"
          className="h-full bg-outline"
          style={{ width: segmentWidth(staleBytes, totalBytes) }}
        />
      </div>

      {errorMessage !== null && (
        <p role="alert" className="text-sm text-error">
          {errorMessage}
        </p>
      )}

      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <p className="text-sm text-on-surface">
          Prune repos older than 30 days
        </p>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            disabled={pendingAction !== null}
            onClick={() => runAction('PRUNE_CACHE', {}, 'prune')}
            className="px-3 py-2 text-sm bg-primary text-on-primary disabled:opacity-60"
          >
            Prune
          </button>
          <button
            type="button"
            disabled={pendingAction !== null}
            onClick={() => {
              if (window.confirm(CLEAR_CACHE_CONFIRM_MESSAGE) !== true) {
                return;
              }
              runAction('CLEAR_CACHE', {}, 'clear');
            }}
            className="px-3 py-2 text-sm bg-surface-container-high text-on-surface disabled:opacity-60"
          >
            Clear all cached repos
          </button>
        </div>
      </div>

      <div className="border-t border-outline-variant/20 pt-4">
        <button
          type="button"
          onClick={() => setIsExpanded((expanded) => !expanded)}
          className="flex w-full items-center justify-between text-left text-sm font-bold text-on-surface"
          aria-expanded={isExpanded}
        >
          <span>Cached repositories</span>
          <span className="font-mono text-[11px] text-on-surface-variant">
            {isExpanded
              ? 'Hide cached repositories'
              : 'Show cached repositories'}
          </span>
        </button>

        {isExpanded && (
          <div className="mt-4 overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-outline-variant/20 text-left font-mono text-[10px] uppercase tracking-widest text-outline">
                  <th className="py-2 pr-4 font-normal">Repo</th>
                  <th className="py-2 pr-4 font-normal">Size</th>
                  <th className="py-2 pr-4 font-normal">Last used</th>
                  <th className="py-2 text-right font-normal">Remove</th>
                </tr>
              </thead>
              <tbody>
                {repos.map((repo) => (
                  <tr
                    key={repo.repo_id}
                    className="border-b border-outline-variant/10 last:border-b-0"
                  >
                    <td className="py-3 pr-4 font-mono text-xs text-on-surface">
                      {repoIdentity(repo.repo_id)}
                    </td>
                    <td className="py-3 pr-4 text-on-surface-variant">
                      {formatBytes(repo.size_bytes)}
                    </td>
                    <td className="py-3 pr-4 text-on-surface-variant">
                      {formatLastUsed(repo.last_used_at)}
                    </td>
                    <td className="py-3 text-right">
                      <button
                        type="button"
                        disabled={pendingAction !== null}
                        aria-label={`Delete cached repo ${repo.repo_id}`}
                        onClick={() =>
                          runAction(
                            'DELETE_CACHE_REPO',
                            { repo_id: repo.repo_id },
                            `delete:${repo.repo_id}`,
                          )
                        }
                        className="material-symbols-outlined text-[20px] text-outline hover:text-error disabled:opacity-60"
                      >
                        delete
                      </button>
                    </td>
                  </tr>
                ))}
                {repos.length === 0 && (
                  <tr>
                    <td
                      colSpan={4}
                      className="py-4 text-sm text-on-surface-variant"
                    >
                      No cached repositories.
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
