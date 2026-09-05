import { isAbsolute } from 'node:path';
import { err, ok, type Result } from 'neverthrow';

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
): Result<ValidOpenInEditorRequest, string> {
  if (typeof request !== 'object' || request === null) {
    return err('Invalid open-in-editor request');
  }
  const { repoRoot, relativePath, line } = request as Record<string, unknown>;
  if (
    typeof repoRoot !== 'string' ||
    repoRoot === '' ||
    !isAbsolute(repoRoot) ||
    repoRoot.includes('\0')
  ) {
    return err('Invalid repository root');
  }
  if (
    typeof relativePath !== 'string' ||
    relativePath === '' ||
    relativePath.includes('\0')
  ) {
    return err('Invalid file path');
  }
  if (
    line !== undefined &&
    !(typeof line === 'number' && Number.isSafeInteger(line) && line >= 1)
  ) {
    return err('Invalid line number');
  }
  return ok({ repoRoot, relativePath, line });
}
