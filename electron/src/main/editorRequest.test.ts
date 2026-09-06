import { describe, expect, it } from 'vitest';
import { validateOpenInEditorRequest } from './editorRequest';

const valid = { repoRoot: '/repo', relativePath: 'src/a.ts', line: 3 };

describe('validateOpenInEditorRequest', () => {
  it('accepts a well-formed request and an omitted line', () => {
    expect(validateOpenInEditorRequest(valid).isOk()).toBe(true);
    expect(
      validateOpenInEditorRequest({ ...valid, line: undefined })._unsafeUnwrap()
        .line,
    ).toBeUndefined();
  });

  it.each([
    ['ex command string', '!rm -rf /'],
    ['zero', 0],
    ['negative', -1],
    ['fractional', 1.5],
    ['NaN', Number.NaN],
    ['null', null],
  ])('rejects line: %s', (_label, line) => {
    expect(validateOpenInEditorRequest({ ...valid, line }).isErr()).toBe(true);
  });

  it('rejects malformed paths', () => {
    expect(
      validateOpenInEditorRequest({ ...valid, relativePath: '' }).isErr(),
    ).toBe(true);
    expect(
      validateOpenInEditorRequest({ ...valid, relativePath: 'a\0b' }).isErr(),
    ).toBe(true);
    expect(
      validateOpenInEditorRequest({ ...valid, repoRoot: 'relative' }).isErr(),
    ).toBe(true);
    expect(validateOpenInEditorRequest(null).isErr()).toBe(true);
  });
});
