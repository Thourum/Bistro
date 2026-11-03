export const COMMIT_TOGGLE_VIEWED_EVENT = 'bistro:commit:toggle-viewed';
export const COMMIT_VIEWED_AND_NEXT_EVENT = 'bistro:commit:viewed-and-next';

export interface CommitPosition {
  index: number;
  total: number;
  prevSha: string | null;
  nextSha: string | null;
}

/** Where `sha` sits in the oldest-first list; null when it is not listed. */
export function commitPosition(
  shas: readonly string[],
  sha: string,
): CommitPosition | null {
  const index = shas.indexOf(sha);
  if (index < 0) return null;
  return {
    index,
    total: shas.length,
    prevSha: shas[index - 1] ?? null,
    nextSha: shas[index + 1] ?? null,
  };
}

const COMMIT_PATH = /^\/prs\/[^/]+\/commits\/([^/]+)$/;

/** Extracts a commit SHA only from the single-commit route. */
export function shaFromCommitPath(pathname: string): string | null {
  return COMMIT_PATH.exec(pathname)?.[1] ?? null;
}
