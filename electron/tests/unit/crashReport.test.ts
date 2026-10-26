import { describe, expect, it, vi } from 'vitest';

vi.mock('../../src/main/updater', () => ({ BISTRO_REPO: 'Thourum/Bistro' }));

import {
  buildCrashIssueUrl,
  MAX_ISSUE_URL_BYTES,
} from '../../src/main/crashReport';

const base = {
  version: '0.1.0',
  platform: 'darwin',
  arch: 'arm64',
  code: 1,
  signal: null,
  home: '/Users/reviewer',
};

describe('buildCrashIssueUrl', () => {
  it.each([
    ['ghp_secret', '[redacted]'],
    ['gho_secret', '[redacted]'],
    ['ghu_secret', '[redacted]'],
    ['ghs_secret', '[redacted]'],
    ['ghr_secret', '[redacted]'],
    ['github_pat_secret', '[redacted]'],
    ['Authorization: Bearer private-token', 'Authorization: [redacted]'],
    [
      'http://localhost/callback?code=private-code&state=private-state',
      'code=[redacted]&state=[redacted]',
    ],
    ['/Users/reviewer/projects/repo', '~/projects/repo'],
  ])('redacts %s before encoding', (line, expected) => {
    const body = new URL(
      buildCrashIssueUrl({ ...base, logs: [line] }),
    ).searchParams.get('body');
    expect(body).toContain(expected);
    expect(body).not.toContain(line);
  });

  it('redacts Windows home paths escaped in quoted Go errors', () => {
    const home = String.raw`C:\Users\reviewer`;
    const line = String.raw`error="open C:\\Users\\reviewer\\sync.db: access denied"`;
    const body = new URL(
      buildCrashIssueUrl({ ...base, home, logs: [line] }),
    ).searchParams.get('body');
    expect(body).toContain(String.raw`~\\sync.db`);
    expect(body).not.toContain('reviewer');
  });

  it('drops oldest lines and preserves readable metadata and the newest tail', () => {
    const logs = Array.from(
      { length: 60 },
      (_, index) => `line-${index} ${'界'.repeat(150)}`,
    );
    const url = buildCrashIssueUrl({ ...base, logs });
    expect(Buffer.byteLength(url, 'utf8')).toBeLessThan(MAX_ISSUE_URL_BYTES);
    const body = new URL(url).searchParams.get('body') ?? '';
    expect(body).toContain('0.1.0');
    expect(body).toContain('darwin / arm64');
    expect(body).not.toContain('line-0 ');
    expect(body).toContain('line-59 ');
  });

  it('can drop a single oversized line without exceeding the URL limit', () => {
    const url = buildCrashIssueUrl({ ...base, logs: ['x'.repeat(20_000)] });
    expect(Buffer.byteLength(url, 'utf8')).toBeLessThan(MAX_ISSUE_URL_BYTES);
  });
});
