import { describe, expect, it } from 'vitest';
import {
  parseScanDevFolderResult,
  parseValidateDevFolderResult,
} from './onboardingResults';

const validationMessage = (response: unknown): string =>
  parseValidateDevFolderResult(response).match(
    () => 'valid',
    (message) => message,
  );

const discoveryCount = (response: unknown): number | string =>
  parseScanDevFolderResult(response).match(
    (result) => result.discovered,
    (message) => message,
  );

describe('development folder result interpretation', () => {
  it('requires successful validation and preserves the canonical path', () => {
    expect(
      parseValidateDevFolderResult({ ok: true, canonical_path: '' }).match(
        (result) => result.canonicalPath,
        (message) => message,
      ),
    ).toBe('');
    for (const response of [null, {}, { ok: true, canonical_path: 1 }]) {
      expect(validationMessage(response)).toBe(
        'Dev folder validation returned an invalid response.',
      );
    }
  });

  it('prefers string errors, then messages, then the validation fallback', () => {
    expect(
      validationMessage({ ok: false, error: '', message: 'message' }),
    ).toBe('');
    expect(validationMessage({ ok: false, error: 1, message: 'message' })).toBe(
      'message',
    );
    expect(validationMessage({ ok: false, message: null })).toBe(
      'Unable to validate that folder.',
    );
  });

  it('prefers a legacy repository list even when the count or ok disagrees', () => {
    expect(
      discoveryCount({
        repositories: [null, 'repo'],
        ok: false,
        discovered: 9,
      }),
    ).toBe(2);
    expect(discoveryCount({ repositories: [], ok: true, discovered: 9 })).toBe(
      0,
    );
  });

  it('accepts numeric counts only on successful scan responses', () => {
    expect(discoveryCount({ ok: true, discovered: -1 })).toBe(-1);
    expect(discoveryCount({ ok: true, discovered: Number.NaN })).toBeNaN();
    expect(
      discoveryCount({ ok: true, discovered: Number.POSITIVE_INFINITY }),
    ).toBe(Number.POSITIVE_INFINITY);
    for (const response of [
      null,
      { ok: false, discovered: 1 },
      { ok: true, discovered: '1' },
    ]) {
      expect(discoveryCount(response)).toBe(
        'Dev folder scan returned an invalid response.',
      );
    }
  });
});
