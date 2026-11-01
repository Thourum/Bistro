/**
 * Machine-generated file detection for the diff view.
 *
 * These files are collapsed by default: their review signal is "it changed, by
 * how much", which the file header already shows. The pattern list is a
 * deliberate constant — not user-configurable until a real gap shows up.
 */
const GENERATED_PATTERNS: readonly RegExp[] = [
  /(^|\/)[^/]*\.lock$/,
  /(^|\/)(pnpm-lock\.yaml|package-lock\.json|npm-shrinkwrap\.json)$/,
  /(^|\/)go\.sum$/,
  /(^|\/)dist\//,
  /\.pb\.go$/,
  /_generated\.[^/]+$/,
  /\.min\.(js|css)$/,
];

/** True when `path` looks machine-generated. Matched against the full path. */
export function isGeneratedFile(path: string): boolean {
  return GENERATED_PATTERNS.some((pattern) => pattern.test(path));
}
