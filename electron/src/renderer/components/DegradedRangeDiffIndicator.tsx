import type React from 'react';
import { FooterCenterSlot } from './FooterSlot';

interface Props {
  /** The engine's `fallback_reason`; surfaced on hover for the curious. */
  reason: string;
}

/**
 * A quiet status note in the footer's center region, shown when a range diff
 * came back `degraded` (system `git range-diff` was absent or too old, so the
 * engine fell back to a plain head-to-head diff).
 *
 * It deliberately lives in the footer, not on the diff surface: the diff is
 * still usable, this is just context. Matches the footer's monospace /
 * uppercase chrome. The full reason is available via the title attribute.
 */
export function DegradedRangeDiffIndicator({
  reason,
}: Props): React.ReactElement {
  return (
    <FooterCenterSlot>
      <span
        role="status"
        title={reason}
        aria-label={`Range diff unavailable: ${reason}`}
        className="flex items-center gap-2 text-[10px] uppercase tracking-widest font-mono text-yellow-600"
      >
        <span aria-hidden="true">⚠</span>
        Range-diff unavailable — showing plain diff
      </span>
    </FooterCenterSlot>
  );
}
