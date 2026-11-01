import { type RepoCounts, RepoLocation, Shell } from '@bindings/services';
import { type Fault, okAsync } from '@thourum/fault';
import type React from 'react';
import { useState } from 'react';
import { call, callWrite, faultReason } from '../services/engine/call';
import type { RepoInfo } from '../types';

type DialogMode = 'actions' | 'moveToCache' | 'removeConfirm';

type RemoveCounts = RepoCounts;

interface RepoRecoveryDialogProps {
  repo: RepoInfo;
  onClose: () => void;
}

const RECOVERY_MESSAGES: Readonly<Record<string, string>> = {
  remote_mismatch: 'That folder belongs to a different repository.',
  permission_denied: 'Bistro does not have permission to use that folder.',
  invalid_path: 'Choose an existing repository folder.',
  path_missing: 'Choose an existing repository folder.',
  clone_failed: 'Bistro could not clone this repository into the cache.',
  auth_failed: 'Authentication failed while recovering this repository.',
};

function userFacingCodeMessage(code: string | null): string {
  return (
    RECOVERY_MESSAGES[code ?? ''] ?? 'Bistro could not recover this repository.'
  );
}

function countsCopy(counts: RemoveCounts): string {
  return `This deletes ${counts.prs} PRs, ${counts.comments} comments, ${counts.threads} threads, ${counts.patchsets} patchsets, and ${counts.actions} queued actions.`;
}

export function RepoRecoveryDialog({
  repo,
  onClose,
}: RepoRecoveryDialogProps): React.ReactElement {
  const [mode, setMode] = useState<DialogMode>('actions');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [removeCounts, setRemoveCounts] = useState<RemoveCounts | null>(null);

  const repoLabel = `${repo.owner}/${repo.repo}`;
  const recoveryMessage = repo.location_error ?? 'Location unavailable.';
  const mismatchHint = repo.location_error_code === 'remote_mismatch';

  const handleError = (fault: Fault): void => {
    setIsSubmitting(false);
    setErrorMessage(userFacingCodeMessage(faultReason(fault)));
  };

  const chooseNewPath = (): void => {
    setIsSubmitting(true);
    setErrorMessage(null);
    call(() => Shell.PickFolder())
      .andThen((path) =>
        path === ''
          ? okAsync(undefined)
          : callWrite(() =>
              RepoLocation.RecoverPath({ repo_id: repo.id, path }),
            ).map(() => {
              onClose();
              return undefined;
            }),
      )
      .match(() => setIsSubmitting(false), handleError);
  };

  const confirmMoveToCache = (): void => {
    setIsSubmitting(true);
    setErrorMessage(null);
    callWrite(() => RepoLocation.RecoverToCache({ repo_id: repo.id })).match(
      () => {
        setIsSubmitting(false);
        onClose();
      },
      handleError,
    );
  };

  const preflightRemove = (): void => {
    setIsSubmitting(true);
    setErrorMessage(null);
    call(() => RepoLocation.RemovePreflight({ repo_id: repo.id })).match(
      (counts) => {
        setIsSubmitting(false);
        setRemoveCounts(counts);
        setMode('removeConfirm');
      },
      handleError,
    );
  };

  const confirmRemove = (): void => {
    setIsSubmitting(true);
    setErrorMessage(null);
    callWrite(() => RepoLocation.Remove({ repo_id: repo.id })).match(() => {
      setIsSubmitting(false);
      onClose();
    }, handleError);
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
      <div className="w-full max-w-xl mx-4 bg-surface-container-lowest border border-primary shadow-ambient p-4">
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
