/**
 * diffFileKey.ts — canonical diff-file identity utility (P0a).
 *
 * Key design decisions:
 * - PRIMARY identity is PATH-BASED: newFile path for added/modified/renamed files,
 *   oldFile path for deleted files.
 * - `a/` / `b/` prefixes from git diff output are stripped.
 * - For renames (oldPath ≠ newPath) the key is `<newPath>` (canonical) with the
 *   old path available in the metadata for display.
 * - An ordinal suffix `::N` is appended ONLY when a diff genuinely contains two
 *   entries with the same canonical path, making it purely a duplicate disambiguator
 *   rather than the primary identity.
 * - No manual URL-encoding — the router layer handles serialisation.
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Minimal interface for the file-identity input.  Matches the fields
 *  available from `parseMultiFileDiff` (DiffSection) and the `ParsedFile`
 *  shape in PierreDiffView.  Only `oldFile` and `newFile` are required. */
export interface DiffFileIdentity {
  /** e.g. `a/src/foo.ts` or `/dev/null` */
  oldFile: string;
  /** e.g. `b/src/foo.ts` or `/dev/null` */
  newFile: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Strip git's `a/` or `b/` path prefix. */
function stripGitPrefix(p: string): string {
  if (p.startsWith('a/') || p.startsWith('b/')) return p.slice(2);
  return p;
}

const DEV_NULL = /^\/dev\/null$/;

/** Return the raw (not yet disambiguated) canonical path for a file. */
function rawCanonicalPath(file: DiffFileIdentity): string {
  const old = stripGitPrefix(file.oldFile ?? '');
  const neu = stripGitPrefix(file.newFile ?? '');

  // Deletion: new file is /dev/null — use old path
  if (DEV_NULL.test(neu) || neu === '') return old;

  // All other cases (add, modify, rename): use new path
  if (DEV_NULL.test(old) || old === '') return neu;
  return neu;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Return the canonical, stable string key for a diff file entry.
 *
 * @param file  - The file identity (oldFile / newFile from the parsed diff).
 * @param files - The full list of file identities in this diff.  Required to
 *                detect duplicate paths and append a disambiguating ordinal
 *                when necessary.
 */
export function canonicalFileKey(
  file: DiffFileIdentity,
  files: DiffFileIdentity[],
): string {
  const path = rawCanonicalPath(file);

  // Count how many entries share this canonical path
  const samePathIndices: number[] = [];
  for (let i = 0; i < files.length; i++) {
    if (rawCanonicalPath(files[i]!) === path) samePathIndices.push(i);
  }

  // No duplicates — no ordinal needed
  if (samePathIndices.length <= 1) return path;

  // Duplicates exist — append ordinal (0-based position within the group)
  const posInGroup = samePathIndices.indexOf(
    files.findIndex((f) => f === file),
  );
  return `${path}::${posInGroup >= 0 ? posInGroup : 0}`;
}

/**
 * Resolve a canonical key back to a file in a parsed list.
 * Returns `undefined` when not found; the caller decides the fallback.
 */
export function findFileByKey<T extends DiffFileIdentity>(
  files: T[],
  key: string,
): T | undefined {
  // Fast path — try direct match (key === rawCanonicalPath, no duplicates)
  for (const f of files) {
    if (rawCanonicalPath(f) === key) return f;
  }

  // Ordinal suffix path — key ends with `::N`
  const ordinalMatch = /^([\s\S]+)::(\d+)$/.exec(key);
  if (ordinalMatch) {
    const basePath = ordinalMatch[1]!;
    const ordinal = parseInt(ordinalMatch[2]!, 10);
    const group = files.filter((f) => rawCanonicalPath(f) === basePath);
    return group[ordinal];
  }

  return undefined;
}

/**
 * Convenience: build all canonical keys for a list in one pass.
 * Returns a parallel array of keys (same order as `files`).
 */
export function buildFileKeys(files: DiffFileIdentity[]): string[] {
  return files.map((f) => canonicalFileKey(f, files));
}
