import React from 'react';
import type { RangeDiffResult } from '../types';
import { formatErrorMessage } from '../utils/error';
import { DegradedRangeDiffIndicator } from './DegradedRangeDiffIndicator';

interface Props {
  result: RangeDiffResult;
}

/**
 * Classify a single raw diff line for coloring. Range-diff output prefixes
 * lines with markers (`+`/`-`/`@@`/` `) much like a unified diff, so the same
 * heuristic reads well for both the real `git range-diff` output and the
 * plain-diff fallback.
 */
function lineClass(line: string): string {
  if (line.startsWith('@@') || line.startsWith('##')) {
    return 'text-primary';
  }
  if (line.startsWith('++') || line.startsWith('--')) {
    // range-diff uses doubled markers for the outer/inner diff layers
    return line.startsWith('++') ? 'text-primary' : 'text-error';
  }
  if (line.startsWith('+')) return 'text-primary';
  if (line.startsWith('-')) return 'text-error';
  if (line.startsWith('diff ') || line.startsWith('index ')) {
    return 'text-outline';
  }
  return 'text-on-surface-variant';
}

/**
 * Renders the raw output of a range diff as a monospace, line-colored block.
 *
 * Honest about its inputs:
 *   - `degraded` → mounts {@link DegradedRangeDiffIndicator} in the footer
 *     (never an in-surface banner) and still renders `raw` when present.
 *   - empty `raw` → a no-content state built from `fallback_reason`, never a
 *     fabricated diff.
 *
 * Scrolls inside `[data-diff-viewport]` so the existing h/j/k/l and page
 * scroll keybindings drive it without extra wiring.
 */
export function RangeDiffView({ result }: Props): React.ReactElement {
  const lines = React.useMemo(
    () => (result.raw.length > 0 ? result.raw.split('\n') : []),
    [result.raw],
  );

  const hasContent = lines.length > 0;

  return (
    <section
      aria-label="Range diff"
      className="h-full flex flex-col bg-surface text-on-surface"
    >
      {result.degraded && (
        <DegradedRangeDiffIndicator reason={result.fallback_reason} />
      )}

      {hasContent ? (
        <div
          data-diff-viewport
          className="flex-1 min-h-0 overflow-auto font-mono text-xs leading-relaxed"
        >
          <table className="w-full border-collapse">
            <tbody>
              {lines.map((line, i) => (
                <tr
                  // biome-ignore lint/suspicious/noArrayIndexKey: diff lines have no stable id; index is the natural order
                  key={i}
                  className="hover:bg-surface-container-lowest"
                >
                  <td className="select-none text-right pr-3 pl-4 w-12 text-outline-variant/60 align-top">
                    {i + 1}
                  </td>
                  <td
                    className={`whitespace-pre pr-4 align-top ${lineClass(line)}`}
                  >
                    {line === '' ? ' ' : line}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <NoContent reason={result.fallback_reason} />
      )}
    </section>
  );
}

function NoContent({ reason }: { reason: string }): React.ReactElement {
  const message =
    reason.length > 0 ? formatErrorMessage(reason) : 'No changes to show.';
  return (
    <div className="flex-1 flex flex-col items-center justify-center gap-3 px-8 text-center">
      <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
        Nothing to compare
      </span>
      <p className="font-mono text-xs text-on-surface-variant max-w-md leading-relaxed">
        {message}
      </p>
    </div>
  );
}
