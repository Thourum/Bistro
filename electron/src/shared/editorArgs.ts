import { err, ok, type Result } from 'neverthrow';

/** `$EDITOR` split into the executable and any leading arguments it carries. */
export interface EditorCommand {
  /** Executable (first whitespace-separated token, e.g. `code`, `/usr/bin/vim`). */
  bin: string;
  /** Remaining tokens, e.g. `['--wait']` for `EDITOR='code --wait'`. */
  leadingArgs: string[];
}

/**
 * Split an `$EDITOR` value on whitespace. The result is always passed to
 * `spawn()` as argv — never through a shell — so quoting is not interpreted.
 */
export function parseEditorCommand(
  editorCommand: string,
): Result<EditorCommand, string> {
  const [bin, ...leadingArgs] = editorCommand.trim().split(/\s+/);
  if (bin === undefined || bin === '') {
    return err('$EDITOR is not set');
  }
  return ok({ bin, leadingArgs });
}

/**
 * Build the CLI argument array to open `file` at `line` in `editorCommand`
 * (the value of `$EDITOR`, e.g. `code`, `vim`, `subl`). Any leading arguments
 * embedded in `$EDITOR` (`code --wait`) are prepended.
 *
 * Editors that support the `-g file:line` convention (VS Code, Cursor) get
 * it; everything else falls back to opening the bare file with no line hint,
 * since editor-specific line flags vary too much to guess safely.
 */
export function buildEditorArgs(
  editorCommand: string,
  file: string,
  line: number | undefined,
): Result<string[], string> {
  return parseEditorCommand(editorCommand).map(({ bin, leadingArgs }) => [
    ...leadingArgs,
    ...targetArgs(editorBinaryName(bin), file, line),
  ]);
}

function targetArgs(
  bin: string,
  file: string,
  line: number | undefined,
): string[] {
  switch (bin) {
    case 'code':
    case 'code-insiders':
    case 'cursor':
      return ['-g', line === undefined ? file : `${file}:${line}`];
    case 'vim':
    case 'nvim':
    case 'vi':
      return line === undefined ? [file] : [`+${line}`, file];
    default:
      return [file];
  }
}

/** Extract the executable name from an $EDITOR binary token, ignoring any path. */
function editorBinaryName(bin: string): string {
  const base = bin.split(/[/\\]/).pop() ?? bin;
  return base.toLowerCase();
}
