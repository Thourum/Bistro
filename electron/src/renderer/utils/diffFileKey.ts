/** Fields needed to identify a parsed diff file. */
export interface DiffFileIdentity {
  oldFile: string;
  newFile: string;
}

function stripGitPrefix(path: string): string {
  return path.startsWith('a/') || path.startsWith('b/') ? path.slice(2) : path;
}

/** Deleted files use their old path; all others use their new path. */
function rawCanonicalPath(file: DiffFileIdentity): string {
  const newPath = stripGitPrefix(file.newFile);
  return newPath === '' || newPath === '/dev/null'
    ? stripGitPrefix(file.oldFile)
    : newPath;
}

/**
 * Build URL keys in diff order, suffixing duplicate paths only when needed.
 * Reserve literal paths before assigning suffixes so a file actually named
 * `foo::1` cannot collide with the second occurrence of `foo`.
 */
export function buildFileKeys(files: readonly DiffFileIdentity[]): string[] {
  const paths = files.map(rawCanonicalPath);
  const pathCounts = new Map<string, number>();
  for (const path of paths) {
    pathCounts.set(path, (pathCounts.get(path) ?? 0) + 1);
  }

  const literalPaths = new Set(paths);
  const usedKeys = new Set<string>();
  const ordinals = new Map<string, number>();
  return paths.map((path) => {
    if (pathCounts.get(path) === 1) {
      usedKeys.add(path);
      return path;
    }

    const ordinal = ordinals.get(path) ?? 0;
    ordinals.set(path, ordinal + 1);
    let key = `${path}::${ordinal}`;
    while (literalPaths.has(key) || usedKeys.has(key)) key += '::0';
    usedKeys.add(key);
    return key;
  });
}

/** Return a file's generated URL key, or an empty key for a non-member. */
export function canonicalFileKey(
  file: DiffFileIdentity,
  files: readonly DiffFileIdentity[],
): string {
  const index = files.indexOf(file);
  return index === -1 ? '' : (buildFileKeys(files)[index] ?? '');
}

/** Resolve only keys actually generated for this diff. */
export function findFileByKey<T extends DiffFileIdentity>(
  files: readonly T[],
  key: string,
): T | undefined {
  const index = buildFileKeys(files).indexOf(key);
  return index === -1 ? undefined : files[index];
}
