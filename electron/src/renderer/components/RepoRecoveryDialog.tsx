import { err, ok, type Result, ResultAsync } from 'neverthrow';
import type React from 'react';
import { useState } from 'react';
import { useWebSocket } from '../hooks/useWebSocket';
import type { RepoInfo } from '../types';
import type { LocationErrorCode } from '../types/repoLocation';

type DialogMode = 'actions' | 'moveToCache' | 'removeConfirm';

type ActionResponse =
  | { ok: true; counts?: RemoveCounts; deleted?: RemoveCounts }
  | { ok: false; code: LocationErrorCode; message: string };

interface RemoveCounts {
  prs: number;
  comments: number;
  threads: number;
  patchsets: number;
  actions: number;
}

interface RepoRecoveryDialogProps {
  repo: RepoInfo;
  onClose: () => void;
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

function isCount(value: unknown): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= 0;
}

function parseCounts(data: unknown): RemoveCounts | null {
  if (!isRecord(data)) {
    return null;
  }

  if (
    isCount(data.prs) &&
    isCount(data.comments) &&
    isCount(data.threads) &&
    isCount(data.patchsets) &&
    isCount(data.actions)
  ) {
    return {
      prs: data.prs,
      comments: data.comments,
      threads: data.threads,
      patchsets: data.patchsets,
      actions: data.actions,
    };
  }

  return null;
}

function parseActionResponse(data: unknown): Result<ActionResponse, string> {
  if (!isRecord(data)) {
    return err('Recovery action returned an invalid response.');
  }

  if (data.ok === true) {
    const counts = parseCounts(data.counts);
    const deleted = parseCounts(data.deleted);
    return ok({
      ok: true,
      ...(counts === null ? {} : { counts }),
      ...(deleted === null ? {} : { deleted }),
    });
  }

  if (
    data.ok === false &&
    isLocationErrorCode(data.code) &&
    typeof data.message === 'string'
  ) {
    return ok({ ok: false, code: data.code, message: data.message });
  }

  return err('Recovery action returned an invalid response.');
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

function userFacingCodeMessage(code: LocationErrorCode): string {
  switch (code) {
    case 'remote_mismatch':
      return 'That folder belongs to a different repository.';
    case 'permission_denied':
      return 'Bistro does not have permission to use that folder.';
    case 'invalid_path':
    case 'path_missing':
      return 'Choose an existing repository folder.';
    case 'clone_failed':
      return 'Bistro could not clone this repository into the cache.';
    case 'auth_failed':
      return 'Authentication failed while recovering this repository.';
    default:
      return 'Bistro could not recover this repository.';
  }
}

function countsCopy(counts: RemoveCounts): string {
  return `This deletes ${counts.prs} PRs, ${counts.comments} comments, ${counts.threads} threads, ${counts.patchsets} patchsets, and ${counts.actions} queued actions.`;
}

function runRequest(
  request: (type: string, data: unknown) => Promise<unknown>,
  type: string,
  payload: unknown,
): ResultAsync<ActionResponse, string> {
  return ResultAsync.fromPromise(
    request(type, payload),
    requestErrorMessage,
  ).andThen(parseActionResponse);
}

export function RepoRecoveryDialog({
  repo,
  onClose,
}: RepoRecoveryDialogProps): React.ReactElement {
  const { request } = useWebSocket();
  const [mode, setMode] = useState<DialogMode>('actions');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [removeCounts, setRemoveCounts] = useState<RemoveCounts | null>(null);

  const repoLabel = `${repo.owner}/${repo.repo}`;
  const recoveryMessage = repo.location_error ?? 'Location unavailable.';
  const mismatchHint = repo.location_error_code === 'remote_mismatch';

  const handleResponse = (response: ActionResponse): boolean => {
    if (response.ok) {
      return true;
    }

    setErrorMessage(userFacingCodeMessage(response.code));
    return false;
  };

  const chooseNewPath = (): void => {
    setIsSubmitting(true);
    setErrorMessage(null);
    ResultAsync.fromPromise(window.electron.pickFolder(), requestErrorMessage)
      .andThen((path) => {
        if (path === null) {
          return ok<ActionResponse, string>({
            ok: false,
            code: 'cancelled',
            message: '',
          });
        }

        return runRequest(request, 'RECOVER_REPO_PATH', {
          repo_id: repo.id,
          path,
        });
      })
      .match(
        (response) => {
          setIsSubmitting(false);
          if (response.ok) {
            onClose();
            return;
          }
          if (response.code !== 'cancelled') {
            handleResponse(response);
          }
        },
        (message) => {
          setIsSubmitting(false);
          setErrorMessage(message);
        },
      );
  };

  const confirmMoveToCache = (): void => {
    setIsSubmitting(true);
    setErrorMessage(null);
    runRequest(request, 'RECOVER_REPO_TO_CACHE', { repo_id: repo.id }).match(
      (response) => {
        setIsSubmitting(false);
        if (handleResponse(response)) {
          onClose();
        }
      },
      (message) => {
        setIsSubmitting(false);
        setErrorMessage(message);
      },
    );
  };

  const preflightRemove = (): void => {
    setIsSubmitting(true);
    setErrorMessage(null);
    runRequest(request, 'REMOVE_REPO_PREFLIGHT', { repo_id: repo.id }).match(
      (response) => {
        setIsSubmitting(false);
        if (!response.ok) {
          handleResponse(response);
          return;
        }
        if (response.counts === undefined) {
          setErrorMessage('Recovery action returned an invalid response.');
          return;
        }
        setRemoveCounts(response.counts);
        setMode('removeConfirm');
      },
      (message) => {
        setIsSubmitting(false);
        setErrorMessage(message);
      },
    );
  };

  const confirmRemove = (): void => {
    setIsSubmitting(true);
    setErrorMessage(null);
    runRequest(request, 'REMOVE_REPO', { repo_id: repo.id }).match(
      (response) => {
        setIsSubmitting(false);
        if (handleResponse(response)) {
          onClose();
        }
      },
      (message) => {
        setIsSubmitting(false);
        setErrorMessage(message);
      },
    );
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Recover repository"
      className="fixed inset-0 z-40 bg-transparent backdrop-blur-[20px] flex items-start justify-center pt-[153px]"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-xl mx-4 bg-surface-container-lowest border border-primary shadow-[0_10px_30px_rgba(25,28,29,0.04)] p-4">
        <div className="flex items-start justify-between gap-4 mb-4">
          <div>
            <h2 className="text-lg font-semibold text-on-surface">
              Recover repository
            </h2>
            <p className="text-sm text-on-surface-variant">
              Repo {repoLabel} could not sync: {recoveryMessage}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="font-mono text-xs text-outline hover:text-on-surface"
          >
            ESC
          </button>
        </div>

        {mismatchHint && (
          <p className="mb-4 text-sm text-error">
            The selected path belongs to another remote.
          </p>
        )}

        {mode === 'moveToCache' && (
          <div className="space-y-4">
            <p className="text-sm text-on-surface-variant">
              Bistro will clone this repository into its managed cache. Your dev
              folder copy will not be changed.
            </p>
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setMode('actions')}>
                Back
              </button>
              <button
                type="button"
                disabled={isSubmitting}
                onClick={confirmMoveToCache}
                className="px-3 py-2 text-sm bg-primary text-on-primary disabled:opacity-60"
              >
                Confirm move to cache
              </button>
            </div>
          </div>
        )}

        {mode === 'removeConfirm' && removeCounts !== null && (
          <div className="space-y-4">
            <p className="text-sm font-semibold text-error">
              {countsCopy(removeCounts)}
            </p>
            <p className="text-sm text-on-surface-variant">
              This removes the repository and its cached PR history from Bistro.
            </p>
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setMode('actions')}>
                Back
              </button>
              <button
                type="button"
                disabled={isSubmitting}
                onClick={confirmRemove}
                className="px-3 py-2 text-sm bg-error text-on-error disabled:opacity-60"
              >
                Confirm remove repo
              </button>
            </div>
          </div>
        )}

        {mode === 'actions' && (
          <div className="grid gap-2">
            <button
              type="button"
              disabled={isSubmitting}
              onClick={chooseNewPath}
              className="border border-primary bg-surface-container px-3 py-3 text-left text-sm text-on-surface hover:bg-surface-container-highest disabled:opacity-60"
            >
              Choose new path
            </button>
            <button
              type="button"
              disabled={isSubmitting}
              onClick={() => setMode('moveToCache')}
              className="border border-outline-variant bg-surface-container px-3 py-3 text-left text-sm text-on-surface hover:bg-surface-container-highest disabled:opacity-60"
            >
              Move to Bistro cache
            </button>
            <button
              type="button"
              disabled={isSubmitting}
              onClick={preflightRemove}
              className="border border-error bg-surface-container px-3 py-3 text-left text-sm text-error hover:bg-surface-container-highest disabled:opacity-60"
            >
              Remove repo and PR history
            </button>
          </div>
        )}

        {errorMessage !== null && (
          <p className="mt-4 text-sm text-error" role="alert">
            {errorMessage}
          </p>
        )}
      </div>
    </div>
  );
}
