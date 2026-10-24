import { describe, expect, it } from 'vitest';
import { parseWsMessage } from '../types/wsSchemas';
import {
  parseScanDevFolderResult,
  parseValidateDevFolderResult,
} from './onboardingResults';

const validationMessage = (response: unknown): string =>
  parseValidateDevFolderResult(response).match(
    () => 'valid',
    (error) => error.message,
  );

const discoveryCount = (response: unknown): number | string =>
  parseScanDevFolderResult(response).match(
    (result) => result.discovered,
    (error) => error.message,
  );

describe('development folder result interpretation', () => {
  it('retains partial scan completion and rejects invalid discovery counts', () => {
    const partial = parseScanDevFolderResult({
      ok: true,
      discovered: 2,
      partial: true,
    });
    expect(partial.isOk()).toBe(true);
    expect(
      partial.match(
        (value) => value,
        () => null,
      ),
    ).toEqual({ discovered: 2, partial: true });
    for (const discovered of [-1, 1.5, Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(
        parseScanDevFolderResult({
          ok: true,
          discovered,
          partial: false,
        }).isErr(),
      ).toBe(true);
    }
    expect(
      parseScanDevFolderResult({ repositories: [] }).match(
        (value) => value,
        () => null,
      ),
    ).toEqual({ discovered: 0, partial: false });
  });

  it('requires successful validation and preserves the canonical path', () => {
    expect(
      parseValidateDevFolderResult({ ok: true, canonical_path: '' }).match(
        (result) => result.canonicalPath,
        (error) => error.message,
      ),
    ).toBe('');
    for (const response of [null, {}, { ok: true, canonical_path: 1 }]) {
      const result = parseValidateDevFolderResult(response);
      expect(result.isErr()).toBe(true);
      if (result.isErr()) {
        expect(result.error.message).toBe(
          'Dev folder validation returned an invalid response.',
        );
        expect(result.error.tag).toBe('VALIDATION_ERROR');
      }
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
    expect(discoveryCount({ ok: true, discovered: 2, partial: false })).toBe(2);
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

  it('surfaces an async scan failure instead of dropping it in transport', () => {
    const failure = {
      ok: false,
      repo_id: '',
      operation: 'scan_dev_folder',
      code: 'scan_failed',
      message: 'Unable to scan development folder.',
    };
    const message = parseWsMessage(
      JSON.stringify({
        id: 'scan-1',
        type: 'SCAN_DEV_FOLDER_RESULT',
        data: failure,
      }),
    );
    expect(message.isOk()).toBe(true);
    expect(discoveryCount(failure)).toBe('Unable to scan development folder.');
  });
});
