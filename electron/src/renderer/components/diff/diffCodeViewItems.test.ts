import { describe, expect, it } from 'vitest';
import type { ExpertiseFileData } from '../../types';
import {
  buildCodeViewItems,
  findUnrenderableDiffFile,
  parseCodeViewFiles,
} from './diffCodeViewItems';
import { splitUnifiedDiff } from './diffPathUtils';

const TWO_FILE_PATCH = `diff --git a/src/first.ts b/src/first.ts
--- a/src/first.ts
+++ b/src/first.ts
@@ -1 +1 @@
-old
+new
diff --git a/src/second.ts b/src/second.ts
--- a/src/second.ts
+++ b/src/second.ts
@@ -1 +1 @@
-old
+new
`;

const FIRST_FILE_EXPERTISE: ExpertiseFileData = {
  file_path: 'src/first.ts',
  experts: [
    {
      author_email: 'expert@example.com',
      author_login: 'expert',
      surviving_lines: 12,
      total_lines: 18,
      commit_score: 0.8,
      is_original: true,
      score: 0.75,
      dormant: false,
    },
  ],
  your_rank: 2,
};

const GENERATED_FILE_PATCH = `diff --git a/pnpm-lock.yaml b/pnpm-lock.yaml
--- a/pnpm-lock.yaml
+++ b/pnpm-lock.yaml
@@ -1 +1 @@
-old
+new
diff --git a/src/first.ts b/src/first.ts
--- a/src/first.ts
+++ b/src/first.ts
@@ -1 +1 @@
-old
+new
`;

function buildItems(
  expertiseByPath: ReadonlyMap<string, ExpertiseFileData> = new Map(),
  expertiseError: string | null = null,
  patch: string = TWO_FILE_PATCH,
  collapseOverrides: readonly string[] = [],
) {
  const { parsedFiles } = parseCodeViewFiles(splitUnifiedDiff(patch));
  return buildCodeViewItems(
    parsedFiles,
    [],
    new Map(),
    null,
    null,
    expertiseByPath,
    expertiseError,
    collapseOverrides,
  );
}

describe('buildCodeViewItems', () => {
  it('changes only the matching item version when expertise arrives', () => {
    const before = buildItems();
    const after = buildItems(
      new Map([[FIRST_FILE_EXPERTISE.file_path, FIRST_FILE_EXPERTISE]]),
    );

    expect(after[0]?.version).not.toBe(before[0]?.version);
    expect(after[1]?.version).toBe(before[1]?.version);
  });

  it('changes the matching item version when rendered expertise changes', () => {
    const before = buildItems(
      new Map([[FIRST_FILE_EXPERTISE.file_path, FIRST_FILE_EXPERTISE]]),
    );
    const after = buildItems(
      new Map([
        [
          FIRST_FILE_EXPERTISE.file_path,
          { ...FIRST_FILE_EXPERTISE, your_rank: 1 },
        ],
      ]),
    );

    expect(after[0]?.version).not.toBe(before[0]?.version);
    expect(after[1]?.version).toBe(before[1]?.version);
  });

  it('changes all item versions when the rendered expertise error changes', () => {
    const before = buildItems();
    const after = buildItems(new Map(), 'Local expertise is unavailable.');

    expect(after.map((item) => item.version)).not.toEqual(
      before.map((item) => item.version),
    );
  });

  it('collapses generated files and leaves sources expanded', () => {
    const items = buildItems(new Map(), null, GENERATED_FILE_PATCH);

    expect(items[0]?.collapsed).toBe(true);
    expect(items[1]?.collapsed).toBe(false);
  });

  it('inverts the default collapse state for an overridden file', () => {
    const files = splitUnifiedDiff(GENERATED_FILE_PATCH);
    const lockKey = files[0]?.canonicalKey;
    const sourceKey = files[1]?.canonicalKey;
    if (lockKey === undefined || sourceKey === undefined) {
      throw new Error('missing canonical keys');
    }
    const items = buildItems(new Map(), null, GENERATED_FILE_PATCH, [
      lockKey,
      sourceKey,
    ]);

    expect(items[0]?.collapsed).toBe(false);
    expect(items[1]?.collapsed).toBe(true);
  });

  it('changes the item version when its collapse state flips', () => {
    const files = splitUnifiedDiff(GENERATED_FILE_PATCH);
    const lockKey = files[0]?.canonicalKey;
    if (lockKey === undefined) throw new Error('missing canonical key');
    const before = buildItems(new Map(), null, GENERATED_FILE_PATCH);
    const after = buildItems(new Map(), null, GENERATED_FILE_PATCH, [lockKey]);

    expect(after[0]?.version).not.toBe(before[0]?.version);
    expect(after[1]?.version).toBe(before[1]?.version);
  });
});

describe('findUnrenderableDiffFile', () => {
  it('returns the selected parse failure for a file excluded from CodeView', () => {
    const files = splitUnifiedDiff(TWO_FILE_PATCH);
    const selectedFile = files[1];
    if (selectedFile === undefined) throw new Error('missing second file');
    const unrenderableFiles = [
      {
        file: selectedFile,
        reason: 'parse-failed' as const,
      },
    ];

    expect(
      findUnrenderableDiffFile(unrenderableFiles, selectedFile.id),
    ).toEqual(unrenderableFiles[0]);
  });

  it('does not classify a renderable selection as a parse failure', () => {
    const files = splitUnifiedDiff(TWO_FILE_PATCH);
    const firstFile = files[0];
    const secondFile = files[1];
    if (firstFile === undefined || secondFile === undefined) {
      throw new Error('missing diff files');
    }
    const unrenderableFiles = [
      {
        file: secondFile,
        reason: 'parse-failed' as const,
      },
    ];

    expect(
      findUnrenderableDiffFile(unrenderableFiles, firstFile.id),
    ).toBeUndefined();
  });
});
