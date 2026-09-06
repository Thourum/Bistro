import { err, ok, type Result } from 'neverthrow';

/** `$EDITOR` split into the executable and any leading arguments it carries. */
export interface EditorCommand {
  /** Executable (first whitespace-separated token, e.g. `code`, `/usr/bin/vim`). */
  bin: string;
  /** Remaining tokens, e.g. `['--wait']` for `EDITOR='code --wait'`. */
  leadingArgs: string[];
}

/**
 * Split an `$EDITOR` value into argv the way a shell would tokenise it:
 * whitespace separates tokens, single/double quotes group, `\` escapes the
 * next character when unquoted. The result is passed straight to
 * `spawn()` — never through a shell — so `"C:\Program Files\...\Code.exe"
 * --wait` yields a single binary token with the spaces intact.
 */
export function parseEditorCommand(
  editorCommand: string,
): Result<EditorCommand, string> {
  const [bin, ...leadingArgs] = tokenize(editorCommand);
  if (bin === undefined || bin === '') {
    return err('$EDITOR is not set');
  }
  return ok({ bin, leadingArgs });
}

function tokenize(input: string): string[] {
  const tokens: string[] = [];
  let current = '';
  let inToken = false;
  let quote: '"' | "'" | null = null;
  for (let i = 0; i < input.length; i++) {
    const ch = input[i] as string;
    if (quote !== null) {
      if (ch === quote) {
        quote = null;
      } else if (
        ch === '\\' &&
        quote === '"' &&
        (input[i + 1] === '"' || input[i + 1] === '\\')
      ) {
        // Inside double quotes only `\"` and `\\` are escapes; other
        // backslashes (Windows paths) are literal.
        current += input[++i];
      } else {
        current += ch;
      }
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      inToken = true;
    } else if (ch === '\\' && i + 1 < input.length) {
      current += input[++i];
      inToken = true;
    } else if (/\s/.test(ch)) {
      if (inToken) tokens.push(current);
      current = '';
      inToken = false;
    } else {
      current += ch;
      inToken = true;
    }
  }
  if (inToken) tokens.push(current);
  return tokens;
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

/**
 * Extract the executable name from an $EDITOR binary token, ignoring any path
 * and a Windows `.exe`/`.cmd`/`.bat` suffix so `Code.exe` matches `code`.
 */
function editorBinaryName(bin: string): string {
  const base = bin.split(/[/\\]/).pop() ?? bin;
  return base.toLowerCase().replace(/\.(exe|cmd|bat)$/, '');
}
