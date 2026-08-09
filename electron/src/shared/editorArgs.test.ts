import { describe, expect, it } from 'vitest';
import { buildEditorArgs } from './editorArgs';

describe('buildEditorArgs', () => {
  it('builds -g file:line for code/cursor', () => {
    const result = buildEditorArgs('code', 'src/foo.ts', 42);
    expect(result.isOk() && result.value).toEqual(['-g', 'src/foo.ts:42']);
  });

  it('builds +line file for vim', () => {
    const result = buildEditorArgs('vim', 'src/foo.ts', 7);
    expect(result.isOk() && result.value).toEqual(['+7', 'src/foo.ts']);
  });

  it('falls back to bare file for unknown editors', () => {
    const result = buildEditorArgs('subl', 'src/foo.ts', 1);
    expect(result.isOk() && result.value).toEqual(['src/foo.ts']);
  });

  it('returns an error Result for an empty editor command', () => {
    const result = buildEditorArgs('', 'src/foo.ts', 1);
    expect(result.isErr()).toBe(true);
  });
});
