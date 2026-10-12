import { err, Fault, ok, type Result, ResultAsync } from '@thourum/fault';
import type React from 'react';
import { useEffect, useRef, useState } from 'react';
import { useWebSocket } from '../hooks/useWebSocket';
import { settingsStore } from '../store/settings';
import type { LocationErrorCode } from '../types/repoLocation';
import { addToast } from './ToastContainer';

type Destination = 'dev_folder' | 'cache';
type Step = 'url' | 'destination';

type AddRepoResponse =
  | { ok: true; source?: string }
  | {
      ok: false;
      code: LocationErrorCode;
      message: string;
      operation?: string;
      repo_id?: string;
    };

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

const ERROR_MESSAGES: Partial<Record<LocationErrorCode, string>> = {
  invalid_url: 'Enter a valid Git repository URL.',
  destination_conflict: 'A repository already exists at that destination.',
  auth_failed: 'Authentication failed while adding the repository.',
  clone_failed: 'Bistro could not clone the repository.',
  permission_denied: 'Bistro does not have permission to use that destination.',
  invalid_path: 'Choose a valid repository destination.',
};

function requestFault(error: unknown): Fault {
  const message =
    error instanceof Error && error.message.trim() !== ''
      ? error.message
      : typeof error === 'string' && error.trim() !== ''
        ? error
        : 'Bistro could not reach the sync engine. Try again.';
  return new Fault(message).withTag('CONNECTION_ERROR').withCause(error);
}

function isRecord(data: unknown): data is Record<string, unknown> {
  return typeof data === 'object' && data !== null;
}

function isLocationErrorCode(code: unknown): code is LocationErrorCode {
  return LOCATION_ERROR_CODES.includes(
    code as (typeof LOCATION_ERROR_CODES)[number],
  );
}

function parseAddRepoResponse(data: unknown): Result<AddRepoResponse, Fault> {
  if (!isRecord(data)) {
    return err(
      new Fault('Add repository returned an invalid response.').withTag(
        'VALIDATION_ERROR',
      ),
    );
  }

  if (data.ok === true) {
    if (data.source !== undefined && typeof data.source !== 'string') {
      return err(
        new Fault('Add repository returned an invalid response.').withTag(
          'VALIDATION_ERROR',
        ),
      );
    }

    return ok(
      data.source === undefined
        ? { ok: true }
        : { ok: true, source: data.source },
    );
  }

  if (
    data.ok === false &&
    isLocationErrorCode(data.code) &&
    typeof data.message === 'string' &&
    (data.operation === undefined || typeof data.operation === 'string') &&
    (data.repo_id === undefined || typeof data.repo_id === 'string')
  ) {
    const response: AddRepoResponse = {
      ok: false,
      code: data.code,
      message: data.message,
      ...(data.operation === undefined ? {} : { operation: data.operation }),
      ...(data.repo_id === undefined ? {} : { repo_id: data.repo_id }),
    };
    return ok(response);
  }

  return err(
    new Fault('Add repository returned an invalid response.').withTag(
      'VALIDATION_ERROR',
    ),
  );
}

interface AddRepoDialogProps {
  onClose: () => void;
}

const FOCUSABLE_SELECTOR =
  'button:not(:disabled), input:not(:disabled), [href], select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';

export function AddRepoDialog({
  onClose,
}: AddRepoDialogProps): React.ReactElement {
  const { request } = useWebSocket();
  const [url, setUrl] = useState('');
  const [step, setStep] = useState<Step>('url');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const devFolderButtonRef = useRef<HTMLButtonElement>(null);
  const cacheButtonRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  // Ignore in-flight WebSocket responses once the dialog is closed/unmounted,
  // so a stale callback cannot toast or close a newly opened overlay.
  const isClosedRef = useRef(false);
  const devFolderPath = settingsStore.state.repositories.devFolderPath;
  const hasDevFolder = devFolderPath !== null && devFolderPath.trim() !== '';

  const close = (): void => {
    isClosedRef.current = true;
    onClose();
  };

  useEffect(() => {
    const opener = document.activeElement;
    return () => {
      isClosedRef.current = true;
      if (opener instanceof HTMLElement) {
        opener.focus();
      }
    };
  }, []);

  useEffect(() => {
    if (step === 'url') {
      const timeoutId = window.setTimeout(() => inputRef.current?.focus(), 0);
      return () => window.clearTimeout(timeoutId);
    }

    const timeoutId = window.setTimeout(() => {
      if (hasDevFolder) {
        devFolderButtonRef.current?.focus();
        return;
      }
      cacheButtonRef.current?.focus();
    }, 0);
    return () => window.clearTimeout(timeoutId);
  }, [hasDevFolder, step]);

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>): void => {
    if (event.key === 'Escape') {
      event.stopPropagation();
      close();
      return;
    }
    if (event.key !== 'Tab' || dialogRef.current === null) {
      return;
    }
    const focusable = Array.from(
      dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
    );
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (first === undefined || last === undefined) {
      event.preventDefault();
      return;
    }
    const active = document.activeElement;
    if (event.shiftKey && (active === first || active === dialogRef.current)) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && active === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const handleResponse = (response: AddRepoResponse): void => {
    if (response.ok) {
      addToast({
        type: 'success',
        message: 'Repository added',
        dismissible: true,
      });
      close();
      return;
    }

    if (response.code === 'destination_required') {
      setStep('destination');
      setErrorMessage(null);
      return;
    }

    setErrorMessage(
      ERROR_MESSAGES[response.code] ?? 'Repository could not be added.',
    );
  };

  const submit = (destination: Destination | null): void => {
    const trimmedUrl = url.trim();
    if (trimmedUrl === '') {
      setErrorMessage('Enter a Git repository URL.');
      return;
    }

    setIsSubmitting(true);
    setErrorMessage(null);

    ResultAsync.fromPromise(
      request('ADD_REPO_BY_URL', {
        url: trimmedUrl,
        destination,
        dev_folder_path: devFolderPath,
      }),
      requestFault,
    )
      .andThen(parseAddRepoResponse)
      .match(
        (response) => {
          if (isClosedRef.current) return;
          setIsSubmitting(false);
          handleResponse(response);
        },
        (error) => {
          if (isClosedRef.current) return;
          setIsSubmitting(false);
          setErrorMessage(error.message);
        },
      );
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Add repository by URL"
      className="fixed inset-0 z-40 bg-transparent backdrop-blur-[20px] flex items-start justify-center pt-[153px]"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) close();
      }}
      onKeyDown={handleKeyDown}
    >
      <div
        ref={dialogRef}
        className="w-full max-w-xl mx-4 bg-surface-container-lowest border border-primary shadow-ambient p-4"
      >
        <div className="flex items-start justify-between gap-4 mb-4">
          <div>
            <h2 className="text-lg font-semibold text-on-surface">
              Add repository by URL
            </h2>
            <p className="text-sm text-on-surface-variant">
              Paste a GitHub repository URL to add it to Bistro.
            </p>
          </div>
          <button
            type="button"
            onClick={close}
            className="font-mono text-xs text-outline hover:text-on-surface"
          >
            ESC
          </button>
        </div>

        {step === 'url' ? (
          <form
            onSubmit={(event) => {
              event.preventDefault();
              submit(null);
            }}
            className="space-y-4"
          >
            <label className="block text-sm font-medium text-on-surface">
              Repository URL
              <input
                ref={inputRef}
                value={url}
                onChange={(event) => setUrl(event.currentTarget.value)}
                className="mt-2 w-full border border-outline-variant bg-surface-container px-3 py-2 font-mono text-sm text-on-surface outline-none focus:border-primary"
                placeholder="https://github.com/owner/repo.git"
              />
            </label>
            {errorMessage !== null && (
              <p className="text-sm text-error" role="alert">
                {errorMessage}
              </p>
            )}
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={close}
                className="px-3 py-2 text-sm text-on-surface-variant hover:text-on-surface"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={isSubmitting}
                className="px-3 py-2 text-sm bg-primary text-on-primary disabled:opacity-60"
              >
                {isSubmitting ? 'Adding…' : 'Add repository'}
              </button>
            </div>
          </form>
        ) : (
          <div className="space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-on-surface">
                Choose a destination
              </h3>
              {!hasDevFolder && (
                <p className="mt-1 text-sm text-on-surface-variant">
                  To add repositories to a dev folder, set one in Settings →
                  Repositories.
                </p>
              )}
            </div>
            <div className="grid gap-2">
              {hasDevFolder && (
                <button
                  ref={devFolderButtonRef}
                  type="button"
                  onClick={() => submit('dev_folder')}
                  disabled={isSubmitting}
                  className="border border-primary bg-surface-container px-3 py-3 text-left text-sm text-on-surface hover:bg-surface-container-highest focus:outline-none focus:ring-2 focus:ring-primary"
                >
                  <span className="block font-semibold">Dev folder</span>
                  <span className="block font-mono text-xs text-on-surface-variant">
                    {devFolderPath}
                  </span>
                </button>
              )}
              <button
                ref={cacheButtonRef}
                type="button"
                onClick={() => submit('cache')}
                disabled={isSubmitting}
                className="border border-outline-variant bg-surface-container px-3 py-3 text-left text-sm text-on-surface hover:bg-surface-container-highest focus:outline-none focus:ring-2 focus:ring-primary"
              >
                <span className="block font-semibold">Bistro cache</span>
                <span className="block text-xs text-on-surface-variant">
                  Clone into Bistro’s managed cache.
                </span>
              </button>
            </div>
            {errorMessage !== null && (
              <p className="text-sm text-error" role="alert">
                {errorMessage}
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
