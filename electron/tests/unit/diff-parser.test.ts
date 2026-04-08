// @vitest-environment node
import { parseDiff, parseMultiFileDiff } from '../../src/renderer/utils/diff-parser';

describe('diff-parser', () => {
  it('parses file headers', () => {
    const diff = `--- a/src/file.ts
+++ b/src/file.ts
@@ -1 +1 @@`;
    const result = parseDiff(diff);
    expect(result.oldFile).toBe('a/src/file.ts');
    expect(result.newFile).toBe('b/src/file.ts');
  });

  it('parses hunk headers', () => {
    const diff = `--- a/file.ts
+++ b/file.ts
@@ -10,7 +10,8 @@`;
    const result = parseDiff(diff);
    expect(result.hunks.length).toBe(1);
    expect(result.hunks[0]?.header).toBe('@@ -10,7 +10,8 @@');
  });

  it('parses added lines', () => {
    const diff = `--- a/file.ts
+++ b/file.ts
@@ -1 +1 @@
+new line`;
    const result = parseDiff(diff);
    expect(result.hunks[0]?.lines[0]?.type).toBe('added');
    expect(result.hunks[0]?.lines[0]?.content).toBe('new line');
  });

  it('parses removed lines', () => {
    const diff = `--- a/file.ts
+++ b/file.ts
@@ -1 +1 @@
-old line`;
    const result = parseDiff(diff);
    expect(result.hunks[0]?.lines[0]?.type).toBe('removed');
    expect(result.hunks[0]?.lines[0]?.content).toBe('old line');
  });

  it('parses context lines', () => {
    const diff = `--- a/file.ts
+++ b/file.ts
@@ -1 +1 @@
 context line`;
    const result = parseDiff(diff);
    expect(result.hunks[0]?.lines[0]?.type).toBe('context');
    expect(result.hunks[0]?.lines[0]?.content).toBe('context line');
  });

  it('tracks line numbers correctly', () => {
    const diff = `--- a/file.ts
+++ b/file.ts
@@ -1,3 +1,4 @@
 line 1
-line 2
+new line 2
 line 3`;
    const result = parseDiff(diff);
    const lines = result.hunks[0]?.lines ?? [];
    expect(lines[0]?.oldLineNumber).toBe(1);
    expect(lines[1]?.oldLineNumber).toBe(2);
    expect(lines[2]?.oldLineNumber).toBeUndefined();
    expect(lines[3]?.oldLineNumber).toBe(3);
    expect(lines[0]?.newLineNumber).toBe(1);
    expect(lines[1]?.newLineNumber).toBeUndefined();
    expect(lines[2]?.newLineNumber).toBe(2);
    expect(lines[3]?.newLineNumber).toBe(3);
  });

  it('returns empty hunks for empty input', () => {
    const result = parseDiff('');
    expect(result.hunks).toHaveLength(0);
    expect(result.oldFile).toBe('');
    expect(result.newFile).toBe('');
  });

  it('skips no-newline-at-end-of-file markers', () => {
    const diff = `--- a/file.ts
+++ b/file.ts
@@ -1 +1 @@
+new line
\\ No newline at end of file`;
    const result = parseDiff(diff);
    expect(result.hunks[0]?.lines).toHaveLength(1);
  });
});

describe('parseMultiFileDiff', () => {
  it('returns empty array for empty input', () => {
    expect(parseMultiFileDiff('')).toHaveLength(0);
    expect(parseMultiFileDiff('   ')).toHaveLength(0);
  });

  it('parses a single-file git diff', () => {
    const diff = `diff --git a/foo.ts b/foo.ts
--- a/foo.ts
+++ b/foo.ts
@@ -1 +1 @@
-old
+new`;
    const sections = parseMultiFileDiff(diff);
    expect(sections).toHaveLength(1);
    expect(sections[0]?.newFile).toBe('b/foo.ts');
    expect(sections[0]?.hunks[0]?.lines).toHaveLength(2);
  });

  it('parses a two-file git diff into two sections', () => {
    const diff = `diff --git a/foo.ts b/foo.ts
--- a/foo.ts
+++ b/foo.ts
@@ -1 +1 @@
-old
+new
diff --git a/bar.ts b/bar.ts
--- a/bar.ts
+++ b/bar.ts
@@ -1 +1 @@
-old
+new`;
    const sections = parseMultiFileDiff(diff);
    expect(sections).toHaveLength(2);
    expect(sections[0]?.newFile).toBe('b/foo.ts');
    expect(sections[1]?.newFile).toBe('b/bar.ts');
  });

  it('handles index lines in git diff', () => {
    const diff = `diff --git a/src/app.ts b/src/app.ts
index abc123..def456 100644
--- a/src/app.ts
+++ b/src/app.ts
@@ -1 +1 @@
-old
+new`;
    const sections = parseMultiFileDiff(diff);
    expect(sections).toHaveLength(1);
    expect(sections[0]?.hunks[0]?.lines).toHaveLength(2);
  });

  it('falls back to git header file names when --- / +++ lines are absent', () => {
    const diff = `diff --git a/README.md b/README.md
new file mode 100644
index 0000000..abc1234`;
    const sections = parseMultiFileDiff(diff);
    expect(sections).toHaveLength(1);
    expect(sections[0]?.oldFile).toBe('a/README.md');
    expect(sections[0]?.newFile).toBe('b/README.md');
  });
});
