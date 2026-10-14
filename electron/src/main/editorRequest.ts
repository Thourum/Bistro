import { isAbsolute } from 'node:path';
import { err, type Fault, ok, type Result, ServiceError } from '@thourum/fault';

export interface ValidOpenInEditorRequest {
  repoRoot: string;
  relativePath: string;
  line: number | undefined;
}

/**
 * Trust boundary for the renderer-supplied open-in-editor payload. `line` in
 * particular is interpolated into editor argv (`+42` for vim) so anything but
 * a positive safe integer is refused — `+!cmd` would otherwise run as an Ex
 * command.
 */
export function validateOpenInEditorRequest(
  request: unknown,
): Result<ValidOpenInEditorRequest, Fault> {
  if (typeof request !== 'object' || request === null) {
    return err(
      ServiceError('VALIDATION_ERROR', 'Invalid open-in-editor request'),
    );
  }
  const { repoRoot, relativePath, line } = request as Record<string, unknown>;
  if (
    typeof repoRoot !== 'string' ||
    repoRoot === '' ||
    !isAbsolute(repoRoot) ||
    repoRoot.includes('\0')
  ) {
    return err(ServiceError('VALIDATION_ERROR', 'Invalid repository root'));
  }
  if (
    typeof relativePath !== 'string' ||
    relativePath === '' ||
    relativePath.includes('\0')
  ) {
    return err(ServiceError('VALIDATION_ERROR', 'Invalid file path'));
  }
  if (
    line !== undefined &&
    !(typeof line === 'number' && Number.isSafeInteger(line) && line >= 1)
  ) {
    return err(ServiceError('VALIDATION_ERROR', 'Invalid line number'));
  }
  return ok({ repoRoot, relativePath, line });
}
