import { fromPromise } from 'neverthrow';
import { useCallback, useRef, useState } from 'react';
import { createLogger } from '../utils/logger';
import { useWebSocket } from './useWebSocket';

const log = createLogger('useOpenInEditor');

interface CheckPRCheckoutData {
  local_path?: string;
  head_sha?: string;
  head_matches?: boolean;
  error?: string;
}

interface CheckoutPRHeadData {
  ok?: boolean;
  local_path?: string;
  reason?: string;
}

/**
 * Dialog state surfaced when the local checkout doesn't match the PR head.
 * `mismatchWarning` renders "may differ from the PR revision" copy.
 */
export interface EditorCheckoutDialog {
  kind: 'checkout-mismatch';
  prId: string;
  repoRoot: string;
  /** Head SHA the mismatch was inspected against; sent back with the
   *  checkout so the engine refuses if the PR was force-pushed meanwhile. */
  headSha: string;
  relativePath: string;
  line: number;
}

interface OpenInEditorState {
  /** Set when the checkout mismatch dialog should be shown. */
  dialog: EditorCheckoutDialog | null;
  /** Toast-worthy error text for a failure that has no dialog affordance. */
  error: string | null;
}

/**
 * Opens a diff file in $EDITOR at a line, verifying the local checkout is at
 * the PR's head SHA first.
 *
 * Flow (spec: docs/superpowers/specs/2026-07-18-phase5-inbox-triage-polish-design.md §5.3):
 *  1. CHECK_PR_CHECKOUT — no local path → error.
 *  2. head_matches → open immediately via the main-process IPC bridge.
 *  3. !head_matches → surface the mismatch dialog; the caller renders three
 *     choices (checkout PR head / open current version / cancel).
 */
export function useOpenInEditor(): {
  state: OpenInEditorState;
  /** Step 1: look up checkout status and either open or surface the dialog. */
  requestOpen: (
    prId: string,
    relativePath: string,
    line: number,
  ) => Promise<void>;
  /** Dialog action: check out the PR head, then open. */
  checkoutAndOpen: () => Promise<void>;
  /** Dialog action: open the working-tree file as-is. */
  openCurrentVersion: () => Promise<void>;
  /** Dialog action: dismiss without opening. */
  cancel: () => void;
  /** True while a dialog action is in flight; the dialog disables its buttons. */
  pending: boolean;
} {
  const { request } = useWebSocket();
  const requestRef = useRef(request);
  requestRef.current = request;
  const [state, setState] = useState<OpenInEditorState>({
    dialog: null,
    error: null,
  });
  // Ref is the synchronous guard (state updates are batched); the state
  // mirrors it for rendering.
  const pendingRef = useRef(false);
  const [pending, setPending] = useState(false);

  /** Run a dialog action once; drop re-entrant calls while one is in flight. */
  const runExclusive = useCallback(
    (action: () => Promise<void>): Promise<void> => {
      if (pendingRef.current) return Promise.resolve();
      pendingRef.current = true;
      setPending(true);
      return action().finally(() => {
        pendingRef.current = false;
        setPending(false);
      });
    },
    [],
  );

  const openViaIPC = useCallback(
    (repoRoot: string, relativePath: string, line: number): Promise<void> =>
      fromPromise(
        window.electron.openInEditor({ repoRoot, relativePath, line }),
        (err) => (err instanceof Error ? err.message : 'Unknown error'),
      ).match(
        (response) => {
          if ('err' in response) {
            setState({ dialog: null, error: response.err });
            return;
          }
          setState({ dialog: null, error: null });
        },
        (err) => {
          log.warn({ err }, 'openInEditor IPC failed');
          setState({ dialog: null, error: err });
        },
      ),
    [],
  );

  const requestOpen = useCallback(
    (prId: string, relativePath: string, line: number): Promise<void> =>
      Promise.resolve(
        fromPromise(
          requestRef.current('CHECK_PR_CHECKOUT', { pr_id: prId }),
          (err) => (err instanceof Error ? err.message : 'Unknown error'),
        ).match(
          async (raw) => {
            const data = raw as CheckPRCheckoutData;
            if (data.error !== undefined) {
              setState({ dialog: null, error: data.error });
              return;
            }
            const repoRoot = data.local_path;
            if (repoRoot === undefined || repoRoot === '') {
              setState({ dialog: null, error: 'no local path' });
              return;
            }
            if (data.head_matches === true) {
              await openViaIPC(repoRoot, relativePath, line);
              return;
            }
            setState({
              dialog: {
                kind: 'checkout-mismatch',
                prId,
                repoRoot,
                headSha: data.head_sha ?? '',
                relativePath,
                line,
              },
              error: null,
            });
          },
          (err) => {
            log.warn({ err, prId }, 'CHECK_PR_CHECKOUT failed');
            setState({ dialog: null, error: err });
          },
        ),
      ),
    [openViaIPC],
  );

  const checkoutAndOpen = useCallback((): Promise<void> => {
    const { dialog } = state;
    if (dialog === null) return Promise.resolve();
    return runExclusive(() =>
      Promise.resolve(
        fromPromise(
          requestRef.current('CHECKOUT_PR_HEAD', {
            pr_id: dialog.prId,
            expected_head_sha: dialog.headSha,
          }),
          (err) => (err instanceof Error ? err.message : 'Unknown error'),
        ).match(
          async (raw) => {
            const data = raw as CheckoutPRHeadData;
            if (data.ok !== true) {
              setState({
                dialog: null,
                error: `Checkout failed: ${data.reason ?? 'unknown error'}`,
              });
              return;
            }
            const repoRoot = data.local_path ?? dialog.repoRoot;
            await openViaIPC(repoRoot, dialog.relativePath, dialog.line);
          },
          (err) => {
            log.warn({ err, prId: dialog.prId }, 'CHECKOUT_PR_HEAD failed');
            setState({ dialog: null, error: err });
          },
        ),
      ),
    );
  }, [state, openViaIPC, runExclusive]);

  const openCurrentVersion = useCallback((): Promise<void> => {
    const { dialog } = state;
    if (dialog === null) return Promise.resolve();
    return runExclusive(() =>
      openViaIPC(dialog.repoRoot, dialog.relativePath, dialog.line),
    );
  }, [state, openViaIPC, runExclusive]);

  const cancel = useCallback((): void => {
    if (pendingRef.current) return;
    setState({ dialog: null, error: null });
  }, []);

  return {
    state,
    requestOpen,
    checkoutAndOpen,
    openCurrentVersion,
    cancel,
    pending,
  };
}
