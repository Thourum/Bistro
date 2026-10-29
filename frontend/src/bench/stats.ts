const DROPPED_FRAME_MULTIPLIER = 1.5;
const P50 = 0.5;
const P95 = 0.95;
const P99 = 0.99;

interface FrameStats {
  readonly p50: number;
  readonly p95: number;
  readonly p99: number;
  readonly droppedFrameShare: number;
}

function percentile(sorted: readonly number[], fraction: number): number {
  return sorted[Math.max(0, Math.ceil(sorted.length * fraction) - 1)] ?? 0;
}

/** Summarizes raw frame deltas using nearest-rank percentiles. */
export function summarizeFrames(
  deltas: readonly number[],
  refreshMs: number,
): FrameStats {
  const sorted = [...deltas].sort((left, right) => left - right);
  return {
    p50: percentile(sorted, P50),
    p95: percentile(sorted, P95),
    p99: percentile(sorted, P99),
    droppedFrameShare:
      deltas.length === 0
        ? 0
        : deltas.filter((delta) => delta > DROPPED_FRAME_MULTIPLIER * refreshMs)
            .length / deltas.length,
  };
}
