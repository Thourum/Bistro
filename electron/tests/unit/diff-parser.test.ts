// @vitest-environment node
import { parseDiff } from '../../src/renderer/utils/diff-parser';

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
