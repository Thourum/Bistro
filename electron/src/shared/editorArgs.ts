import { err, ok, type Result } from 'neverthrow';

/**
 * Build the CLI argument array to open `file` at `line` in `editorCommand`
 * (the value of `$EDITOR`, e.g. `code`, `vim`, `subl`).
 *
 * Editors that support the `-g file:line` convention (VS Code, Cursor) get
 * it; everything else falls back to opening the bare file with no line hint,
 * since editor-specific line flags vary too much to guess safely.
 */
export function buildEditorArgs(
  editorCommand: string,
  file: string,
  line: number,
): Result<string[], string> {
  if (editorCommand.trim() === '') {
    return err('$EDITOR is not set');
  }
  const bin = editorBinaryName(editorCommand);
  switch (bin) {
    case 'code':
    case 'code-insiders':
    case 'cursor':
      return ok(['-g', `${file}:${line}`]);
    case 'vim':
    case 'nvim':
    case 'vi':
      return ok([`+${line}`, file]);
    default:
      return ok([file]);
  }
}

/** Extract the executable name from an $EDITOR value, ignoring any path or args. */
function editorBinaryName(editorCommand: string): string {
  const firstToken = editorCommand.trim().split(/\s+/)[0] ?? '';
  const base = firstToken.split(/[/\\]/).pop() ?? firstToken;
  return base.toLowerCase();
}
