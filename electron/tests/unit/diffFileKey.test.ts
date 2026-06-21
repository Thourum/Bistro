// @vitest-environment node
import {
  canonicalFileKey,
  findFileByKey,
  buildFileKeys,
  type DiffFileIdentity,
} from '../../src/renderer/utils/diffFileKey';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function f(oldFile: string, newFile: string): DiffFileIdentity {
  return { oldFile, newFile };
}

// ---------------------------------------------------------------------------
// canonicalFileKey
// ---------------------------------------------------------------------------

describe('canonicalFileKey', () => {
  it('returns the new path (b/ side) for a normal modified file', () => {
    const files = [f('a/src/foo.ts', 'b/src/foo.ts')];
    expect(canonicalFileKey(files[0]!, files)).toBe('src/foo.ts');
  });

  it('strips git a/ and b/ prefixes', () => {
    const files = [f('a/lib/util.js', 'b/lib/util.js')];
    expect(canonicalFileKey(files[0]!, files)).toBe('lib/util.js');
  });

  it('handles paths with spaces', () => {
    const files = [f('a/my component/App.tsx', 'b/my component/App.tsx')];
    expect(canonicalFileKey(files[0]!, files)).toBe('my component/App.tsx');
  });

  it('returns the new path for a rename', () => {
    const files = [f('a/old-name.ts', 'b/new-name.ts')];
    expect(canonicalFileKey(files[0]!, files)).toBe('new-name.ts');
  });

  it('returns the old path for a deletion (/dev/null new side)', () => {
    const files = [f('a/deleted.ts', '/dev/null')];
    expect(canonicalFileKey(files[0]!, files)).toBe('deleted.ts');
  });

  it('returns the new path for an addition (/dev/null old side)', () => {
    const files = [f('/dev/null', 'b/added.ts')];
    expect(canonicalFileKey(files[0]!, files)).toBe('added.ts');
  });

  it('appends ordinal only for genuine duplicate paths', () => {
    const a = f('a/dup.ts', 'b/dup.ts');
    const b = f('a/dup.ts', 'b/dup.ts');
    const files = [a, b];
    expect(canonicalFileKey(a, files)).toBe('dup.ts::0');
    expect(canonicalFileKey(b, files)).toBe('dup.ts::1');
  });

  it('does NOT append ordinal for unique paths', () => {
    const files = [f('a/foo.ts', 'b/foo.ts'), f('a/bar.ts', 'b/bar.ts')];
    expect(canonicalFileKey(files[0]!, files)).toBe('foo.ts');
    expect(canonicalFileKey(files[1]!, files)).toBe('bar.ts');
  });

  it('is stable under reordering (same key regardless of position in list)', () => {
    const a = f('a/alpha.ts', 'b/alpha.ts');
    const b = f('a/beta.ts', 'b/beta.ts');

    const key1 = canonicalFileKey(a, [a, b]);
    const key2 = canonicalFileKey(a, [b, a]);
    expect(key1).toBe(key2);
    expect(key1).toBe('alpha.ts');
  });
});

// ---------------------------------------------------------------------------
// findFileByKey
// ---------------------------------------------------------------------------

describe('findFileByKey', () => {
  it('finds a file by its canonical key (normal path)', () => {
    const fileA = f('a/src/foo.ts', 'b/src/foo.ts');
    const fileB = f('a/src/bar.ts', 'b/src/bar.ts');
    const files = [fileA, fileB];
    expect(findFileByKey(files, 'src/foo.ts')).toBe(fileA);
    expect(findFileByKey(files, 'src/bar.ts')).toBe(fileB);
  });

  it('returns undefined for an unknown key', () => {
    const files = [f('a/foo.ts', 'b/foo.ts')];
    expect(findFileByKey(files, 'unknown.ts')).toBeUndefined();
  });

  it('finds a file with spaces in its path', () => {
    const fileA = f('a/my dir/App.tsx', 'b/my dir/App.tsx');
    const files = [fileA];
    expect(findFileByKey(files, 'my dir/App.tsx')).toBe(fileA);
  });

  it('finds a renamed file by its new (canonical) path', () => {
    const renamed = f('a/old.ts', 'b/new.ts');
    const files = [renamed];
    expect(findFileByKey(files, 'new.ts')).toBe(renamed);
  });

  it('finds a deleted file by its old path', () => {
    const deleted = f('a/gone.ts', '/dev/null');
    const files = [deleted];
    expect(findFileByKey(files, 'gone.ts')).toBe(deleted);
  });

  it('resolves ordinal keys for duplicate paths', () => {
    const a = f('a/dup.ts', 'b/dup.ts');
    const b = f('a/dup.ts', 'b/dup.ts');
    const files = [a, b];
    expect(findFileByKey(files, 'dup.ts::0')).toBe(a);
    expect(findFileByKey(files, 'dup.ts::1')).toBe(b);
  });

  it('round-trips: findFileByKey(canonicalFileKey(f)) === f', () => {
    const files = [
      f('a/src/app.ts', 'b/src/app.ts'),
      f('a/components/Nav.tsx', 'b/components/Nav.tsx'),
      f('a/deleted.ts', '/dev/null'),
      f('/dev/null', 'b/added.ts'),
      f('a/old-name.ts', 'b/new-name.ts'),
    ];
    for (const file of files) {
      const key = canonicalFileKey(file, files);
      expect(findFileByKey(files, key)).toBe(file);
    }
  });

  it('round-trips for duplicate paths', () => {
    const a = f('a/dup.ts', 'b/dup.ts');
    const b = f('a/dup.ts', 'b/dup.ts');
    const files = [a, b];
    expect(findFileByKey(files, canonicalFileKey(a, files))).toBe(a);
    expect(findFileByKey(files, canonicalFileKey(b, files))).toBe(b);
  });
});

// ---------------------------------------------------------------------------
// buildFileKeys
// ---------------------------------------------------------------------------

describe('buildFileKeys', () => {
  it('returns a parallel array of keys for each file', () => {
    const files = [
      f('a/foo.ts', 'b/foo.ts'),
      f('a/bar.ts', 'b/bar.ts'),
    ];
    expect(buildFileKeys(files)).toEqual(['foo.ts', 'bar.ts']);
  });

  it('assigns ordinals only to duplicates', () => {
    const files = [
      f('a/dup.ts', 'b/dup.ts'),
      f('a/unique.ts', 'b/unique.ts'),
      f('a/dup.ts', 'b/dup.ts'),
    ];
    expect(buildFileKeys(files)).toEqual(['dup.ts::0', 'unique.ts', 'dup.ts::1']);
  });
});

// ---------------------------------------------------------------------------
// Round-trip through PierreDiffView's key authority (diffFileKey.ts)
//
// These tests use the SAME buildFileKeys / findFileByKey functions that
// PierreDiffView uses (after buildCanonicalPathKeys was deleted).
// They verify that a key generated for rename / space / duplicate paths
// resolves back to the correct file — proving one keyspace across
// generation (PierreDiffView → store.diffFileKeys) and resolution
// (PierreDiffView selectedFileKey effect → active file).
// ---------------------------------------------------------------------------

describe('PierreDiffView key authority round-trips', () => {
  it('rename: key is new path, resolves back to the renamed file', () => {
    const renamed = f('a/old-name.ts', 'b/new-name.ts');
    const files = [renamed];
    const [key] = buildFileKeys(files);
    expect(key).toBe('new-name.ts');
    expect(findFileByKey(files, key!)).toBe(renamed);
  });

  it('deletion: key is old path, resolves back to the deleted file', () => {
    const deleted = f('a/gone.ts', '/dev/null');
    const files = [deleted];
    const [key] = buildFileKeys(files);
    expect(key).toBe('gone.ts');
    expect(findFileByKey(files, key!)).toBe(deleted);
  });

  it('path with spaces: key includes spaces, resolves correctly', () => {
    const spaced = f('a/my dir/App.tsx', 'b/my dir/App.tsx');
    const files = [spaced];
    const [key] = buildFileKeys(files);
    expect(key).toBe('my dir/App.tsx');
    expect(findFileByKey(files, key!)).toBe(spaced);
  });

  it('duplicate paths: ordinal keys each resolve to the correct file', () => {
    const a = f('a/dup.ts', 'b/dup.ts');
    const unique = f('a/unique.ts', 'b/unique.ts');
    const b = f('a/dup.ts', 'b/dup.ts');
    const files = [a, unique, b];
    const keys = buildFileKeys(files);
    expect(keys).toEqual(['dup.ts::0', 'unique.ts', 'dup.ts::1']);
    expect(findFileByKey(files, 'dup.ts::0')).toBe(a);
    expect(findFileByKey(files, 'unique.ts')).toBe(unique);
    expect(findFileByKey(files, 'dup.ts::1')).toBe(b);
  });

  it('rename + space + duplicate: full mixed round-trip', () => {
    const files = [
      f('a/old name.ts', 'b/new name.ts'), // rename + space → key is 'new name.ts'
      f('a/dup.ts', 'b/dup.ts'), // first dup
      f('/dev/null', 'b/added.ts'), // addition
      f('a/dup.ts', 'b/dup.ts'), // second dup
      f('a/removed.ts', '/dev/null'), // deletion
    ];
    const keys = buildFileKeys(files);
    for (const [i, file] of files.entries()) {
      expect(findFileByKey(files, keys[i]!)).toBe(file);
    }
  });
});
