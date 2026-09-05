import { describe, expect, it } from 'vitest';
import { buildEditorArgs, parseEditorCommand } from './editorArgs';

describe('buildEditorArgs', () => {
  it('builds -g file:line for code/cursor', () => {
    const result = buildEditorArgs('code', 'src/foo.ts', 42);
    expect(result.isOk() && result.value).toEqual(['-g', 'src/foo.ts:42']);
  });

  it('builds +line file for vim', () => {
    const result = buildEditorArgs('vim', 'src/foo.ts', 7);
    expect(result.isOk() && result.value).toEqual(['+7', 'src/foo.ts']);
  });

  it('omits the line argument when line is undefined', () => {
    expect(
      buildEditorArgs('vim', 'src/foo.ts', undefined)._unsafeUnwrap(),
    ).toEqual(['src/foo.ts']);
    expect(
      buildEditorArgs('code', 'src/foo.ts', undefined)._unsafeUnwrap(),
    ).toEqual(['-g', 'src/foo.ts']);
  });

  it('prepends leading args embedded in $EDITOR', () => {
    const result = buildEditorArgs('code --wait', 'src/foo.ts', 3);
    expect(result.isOk() && result.value).toEqual([
      '--wait',
      '-g',
      'src/foo.ts:3',
    ]);
  });

  it('falls back to bare file for unknown editors', () => {
    const result = buildEditorArgs('subl', 'src/foo.ts', 1);
    expect(result.isOk() && result.value).toEqual(['src/foo.ts']);
  });

  it('returns error Result for empty editor command', () => {
    const result = buildEditorArgs('', 'src/foo.ts', 1);
    expect(result.isErr()).toBe(true);
  });
});

describe('parseEditorCommand', () => {
  it('splits binary from leading args', () => {
    expect(
      parseEditorCommand(' /usr/bin/nvim -u NONE ')._unsafeUnwrap(),
    ).toEqual({
      bin: '/usr/bin/nvim',
      leadingArgs: ['-u', 'NONE'],
    });
  });

  it('rejects blank input', () => {
    expect(parseEditorCommand('   ').isErr()).toBe(true);
  });
});
