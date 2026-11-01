import { describe, expect, it } from 'vitest';
import {
  extractFileIdentity,
  extractFilePath,
  splitUnifiedDiff,
} from './diffPathUtils';

const PATCH_BODY = '@@ -1 +1 @@\n-old\n+new\n';

describe('extractFilePath', () => {
  it('returns the new path for a rename', () => {
    const patch = [
      'diff --git a/src/old-name.ts b/src/new-name.ts',
      'similarity index 100%',
      'rename from src/old-name.ts',
      'rename to src/new-name.ts',
    ].join('\n');

    expect(extractFilePath(patch)).toBe('src/new-name.ts');
  });

  it('returns the old path for a deletion', () => {
    const patch = [
      'diff --git a/src/removed.ts b/src/removed.ts',
      'deleted file mode 100644',
      '--- a/src/removed.ts',
      '+++ /dev/null',
      PATCH_BODY,
    ].join('\n');

    expect(extractFilePath(patch)).toBe('src/removed.ts');
  });

  it('returns the path from same-path git headers', () => {
    const patch = [
      'diff --git a/src/current.ts b/src/current.ts',
      '--- a/src/current.ts',
      '+++ b/src/current.ts',
      PATCH_BODY,
    ].join('\n');

    expect(extractFilePath(patch)).toBe('src/current.ts');
  });

  it('returns the new quoted path', () => {
    const patch = [
      'diff --git "a/src/old name.ts" "b/src/new name.ts"',
      'similarity index 100%',
      'rename from src/old name.ts',
      'rename to src/new name.ts',
    ].join('\n');

    expect(extractFilePath(patch)).toBe('src/new name.ts');
  });
});

describe('extractFileIdentity', () => {
  it('retains prefixes for quoted rename paths with spaces', () => {
    const patch = 'diff --git "a/src/old name.ts" "b/src/new name.ts"';

    expect(extractFileIdentity(patch)).toEqual({
      oldFile: 'a/src/old name.ts',
      newFile: 'b/src/new name.ts',
    });
  });

  it('retains git prefixes and marks a deletion as new /dev/null', () => {
    const patch = [
      'diff --git a/src/removed.ts b/src/removed.ts',
      'deleted file mode 100644',
      '--- a/src/removed.ts',
      '+++ /dev/null',
      PATCH_BODY,
    ].join('\n');

    expect(extractFileIdentity(patch)).toEqual({
      oldFile: 'a/src/removed.ts',
      newFile: '/dev/null',
    });
  });
});

describe('splitUnifiedDiff', () => {
  it('keeps the new path as the parsed path for a rename', () => {
    const patch = [
      'diff --git a/src/old-name.ts b/src/new-name.ts',
      'similarity index 100%',
      'rename from src/old-name.ts',
      'rename to src/new-name.ts',
      '--- a/src/old-name.ts',
      '+++ b/src/new-name.ts',
      PATCH_BODY,
    ].join('\n');

    expect(splitUnifiedDiff(patch)).toMatchObject([
      {
        path: 'src/new-name.ts',
        oldFile: 'a/src/old-name.ts',
        newFile: 'b/src/new-name.ts',
      },
    ]);
  });
});
