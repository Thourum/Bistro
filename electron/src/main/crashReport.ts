import { BISTRO_REPO } from './updater';

export const REPORT_LOG_LINES = 60;
export const MAX_ISSUE_URL_BYTES = 7_500;

export interface CrashReportInput {
  readonly version: string;
  readonly platform: string;
  readonly arch: string;
  readonly code: number | null;
  readonly signal: string | null;
  readonly home: string;
  readonly logs: readonly string[];
}

function redactLog(line: string, home: string): string {
  const redacted = line
    .replace(
      /\b(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+)\b/g,
      '[redacted]',
    )
    .replace(/(Authorization:\s*)[^\r\n]+/gi, '$1[redacted]')
    .replace(/([?&](?:code|state)=)[^&#\s]*/gi, '$1[redacted]');
  if (home === '') return redacted;
  return redacted
    .split(home.replaceAll('\\', '\\\\'))
    .join('~')
    .split(home)
    .join('~');
}

export function buildCrashIssueUrl(input: Readonly<CrashReportInput>): string {
  const logs = input.logs
    .slice(-REPORT_LOG_LINES)
    .map((line) => redactLog(line, input.home));
  const exit = input.signal ?? String(input.code ?? 'unknown');
  const makeUrl = (tail: readonly string[]): string => {
    // Tildes avoid engine output containing backticks closing the fenced block.
    const body = `Version: ${input.version}\nOS / architecture: ${input.platform} / ${input.arch}\nEngine exit: ${exit}\n\n~~~~text\n${tail.join('\n').replaceAll('~~~~', '[log fence]')}\n~~~~\n`;
    const query = new URLSearchParams({ title: 'Sync engine stopped', body });
    return `https://github.com/${BISTRO_REPO}/issues/new?${query}`;
  };
  let oldest = 0;
  let url = makeUrl(logs);
  while (
    Buffer.byteLength(url, 'utf8') >= MAX_ISSUE_URL_BYTES &&
    oldest < logs.length
  ) {
    oldest += 1;
    url = makeUrl(logs.slice(oldest));
  }
  return url;
}
