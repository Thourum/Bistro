import { realpath, stat } from 'node:fs/promises';
import { isAbsolute, join, resolve, sep } from 'node:path';
import {
  errAsync,
  type Fault,
  fromPromise,
  okAsync,
  type ResultAsync,
  ServiceError,
} from '@thourum/fault';

export interface EditorTargetRequest {
  /** Absolute checkout root. Must be a git checkout the engine reported. */
  repoRoot: string;
  /** Repo-relative path of the file to open. */
  relativePath: string;
}

/** Rejection copy is shared so callers report a single, non-leaky message. */
const OUTSIDE_REPO = 'Access denied: path outside repository';
const NOT_A_CHECKOUT = 'Access denied: repo root is not a git checkout';
const NOT_ABSOLUTE = 'Access denied: repo root must be an absolute path';

/**
 * Canonicalises `repoRoot` and the requested target, then enforces that the
 * target really lives inside the checkout.
 *
 * Lexical `resolve` alone is not an authorization boundary: a repo-relative
 * symlink (file or directory) can point anywhere on disk and still pass a
 * `startsWith(root)` prefix test. Both sides are therefore resolved with
 * `fs.realpath`, which follows every symlink component, before comparison.
 *
 * The root is additionally required to contain a `.git` entry, so a caller
 * cannot nominate an arbitrary directory (e.g. `$HOME`) as "the repository"
 * and read anything beneath it.
 */
export function resolveEditorTarget(
  request: EditorTargetRequest,
): ResultAsync<string, Fault> {
  const { repoRoot, relativePath } = request;

  if (repoRoot === '' || !isAbsolute(repoRoot)) {
    return errAsync(ServiceError('AUTHORIZATION_ERROR', NOT_ABSOLUTE));
  }
  if (relativePath === '' || isAbsolute(relativePath)) {
    return errAsync(ServiceError('AUTHORIZATION_ERROR', OUTSIDE_REPO));
  }

  return canonicalise(repoRoot, NOT_A_CHECKOUT)
    .andThen((canonicalRoot) =>
      isGitCheckout(canonicalRoot).andThen((isCheckout) =>
        isCheckout
          ? okAsync(canonicalRoot)
          : errAsync(ServiceError('AUTHORIZATION_ERROR', NOT_A_CHECKOUT)),
      ),
    )
    .andThen((canonicalRoot) =>
      canonicalise(resolve(canonicalRoot, relativePath), OUTSIDE_REPO).andThen(
        (canonicalTarget) =>
          contains(canonicalRoot, canonicalTarget)
            ? okAsync(canonicalTarget)
            : errAsync(ServiceError('AUTHORIZATION_ERROR', OUTSIDE_REPO)),
      ),
    );
}

/** realpath, mapping any failure (missing path, loop, EACCES) to `reason`. */
function canonicalise(
  path: string,
  reason: string,
): ResultAsync<string, Fault> {
  return fromPromise(realpath(path), (cause) =>
    ServiceError('AUTHORIZATION_ERROR', reason).withCause(cause),
  );
}

function isGitCheckout(canonicalRoot: string): ResultAsync<boolean, Fault> {
  return fromPromise(stat(join(canonicalRoot, '.git')), (cause) =>
    ServiceError('AUTHORIZATION_ERROR', NOT_A_CHECKOUT).withCause(cause),
  )
    .map(() => true)
    .orElse(() => okAsync(false));
}

/**
 * True when `target` is `root` itself or a descendant of it. The `sep` suffix
 * keeps sibling directories that merely share a prefix (`/repo-evil` vs
 * `/repo`) out.
 */
function contains(root: string, target: string): boolean {
  return target === root || target.startsWith(root + sep);
}
