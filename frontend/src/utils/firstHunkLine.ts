/**
 * Returns the new-side (`+`) starting line number of a single-file patch's
 * first hunk, or 1 when the patch has no hunks. Used by `:open in editor`
 * (spec §5.3) as the fallback line when no inline comment is selected.
 */
export function firstHunkAdditionLine(patch: string): number {
  const match = /^@@ -\d+(?:,\d+)? \+(\d+)/m.exec(patch);
  if (match === null) return 1;
  const line = Number(match[1]);
  return Number.isFinite(line) && line > 0 ? line : 1;
}
