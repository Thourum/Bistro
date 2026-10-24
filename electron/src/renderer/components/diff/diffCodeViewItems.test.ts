import { describe, expect, it } from 'vitest';
import type { ExpertiseFileData } from '../../types';
import {
  buildCodeViewItems,
  findUnrenderableDiffFile,
  isBinaryPatch,
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
  return buildCodeViewItems({
    parsedCodeViewFiles: parsedFiles,
    threads: [],
    commentsByThread: new Map(),
    activeAnnotationKey: null,
    replyingToThreadId: null,
    expertiseByPath,
    expertiseError,
    collapseOverrides,
  });
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

  it('retains the version for an unchanged patch and changes it for a changed patch', () => {
    const files = splitUnifiedDiff(TWO_FILE_PATCH);
    const before = parseCodeViewFiles(files).parsedFiles;
    const same = parseCodeViewFiles(files).parsedFiles;
    const firstFile = files[0];
    if (firstFile === undefined) throw new Error('missing first file');
    const changed = parseCodeViewFiles([
      { ...firstFile, patch: firstFile.patch.replace('+new', '+updated') },
      ...files.slice(1),
    ]).parsedFiles;
    const options = {
      threads: [],
      commentsByThread: new Map(),
      activeAnnotationKey: null,
      replyingToThreadId: null,
      expertiseByPath: new Map(),
      expertiseError: null,
    };

    const beforeVersions = buildCodeViewItems({
      ...options,
      parsedCodeViewFiles: before,
    });
    const sameVersions = buildCodeViewItems({
      ...options,
      parsedCodeViewFiles: same,
    });
    const changedVersions = buildCodeViewItems({
      ...options,
      parsedCodeViewFiles: changed,
    });

    expect(sameVersions.map((item) => item.version)).toEqual(
      beforeVersions.map((item) => item.version),
    );
    expect(changedVersions[0]?.version).not.toBe(beforeVersions[0]?.version);
    expect(changedVersions[1]?.version).toBe(beforeVersions[1]?.version);
  });

  it('changes all item versions when the rendered expertise error changes', () => {
    const before = buildItems();
    const after = buildItems(new Map(), 'Local expertise is unavailable.');

    expect(after.map((item) => item.version)).not.toEqual(
      before.map((item) => item.version),
    );
  });

  it('collapses generated files by default and inverts on override', () => {
    const files = splitUnifiedDiff(GENERATED_FILE_PATCH);
    const lockKey = files[0]?.canonicalKey;
    const sourceKey = files[1]?.canonicalKey;
    expect(lockKey).toBeDefined();
    expect(sourceKey).toBeDefined();
    if (lockKey === undefined || sourceKey === undefined) return;

    expect(
      buildItems(new Map(), null, GENERATED_FILE_PATCH).map((i) => i.collapsed),
    ).toEqual([true, false]);
    expect(
      buildItems(new Map(), null, GENERATED_FILE_PATCH, [
        lockKey,
        sourceKey,
      ]).map((i) => i.collapsed),
    ).toEqual([false, true]);
  });

  it('changes the item version when its collapse state flips', () => {
    const files = splitUnifiedDiff(GENERATED_FILE_PATCH);
    const lockKey = files[0]?.canonicalKey;
    expect(lockKey).toBeDefined();
    if (lockKey === undefined) return;
    const before = buildItems(new Map(), null, GENERATED_FILE_PATCH);
    const after = buildItems(new Map(), null, GENERATED_FILE_PATCH, [lockKey]);

    expect(after[0]?.version).not.toBe(before[0]?.version);
    expect(after[1]?.version).toBe(before[1]?.version);
  });
});

const BINARY_PATCH = `diff --git a/logo.png b/logo.png
index 1111111..2222222 100644
Binary files a/logo.png and b/logo.png differ
diff --git a/src/first.ts b/src/first.ts
--- a/src/first.ts
+++ b/src/first.ts
@@ -1 +1 @@
-old
+new
`;

describe('isBinaryPatch', () => {
  it('detects the "Binary files ... differ" marker', () => {
    expect(
      isBinaryPatch(
        'diff --git a/a.png b/a.png\nBinary files a/a.png and b/a.png differ\n',
      ),
    ).toBe(true);
  });

  it('detects a GIT binary patch payload', () => {
    expect(
      isBinaryPatch(
        'diff --git a/a.png b/a.png\nGIT binary patch\nliteral 4\n',
      ),
    ).toBe(true);
  });

  it('does not flag a text patch', () => {
    expect(isBinaryPatch(TWO_FILE_PATCH)).toBe(false);
  });
});

describe('parseCodeViewFiles', () => {
  it('classifies a binary file as unrenderable and keeps text files renderable', () => {
    const { parsedFiles, unrenderableFiles } = parseCodeViewFiles(
      splitUnifiedDiff(BINARY_PATCH),
    );

    expect(unrenderableFiles).toHaveLength(1);
    expect(unrenderableFiles[0]?.reason).toBe('binary');
    expect(unrenderableFiles[0]?.file.path).toBe('logo.png');
    expect(parsedFiles.map(({ file }) => file.path)).toEqual(['src/first.ts']);
  });

  it('classifies an oversized file as too-large with its original size', () => {
    const headerOnly = `diff --git a/huge.c b/huge.c
diff --git a/src/first.ts b/src/first.ts
--- a/src/first.ts
+++ b/src/first.ts
@@ -1 +1 @@
-old
+new
`;
    const { parsedFiles, unrenderableFiles } = parseCodeViewFiles(
      splitUnifiedDiff(headerOnly),
      new Map([['huge.c', 600 * 1024]]),
    );

    expect(unrenderableFiles).toEqual([
      expect.objectContaining({ reason: 'too-large', patchBytes: 600 * 1024 }),
    ]);
    expect(unrenderableFiles[0]?.file.path).toBe('huge.c');
    expect(parsedFiles.map(({ file }) => file.path)).toEqual(['src/first.ts']);
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
