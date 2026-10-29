import React from 'react';
import type { Patchset } from '../types';

interface Props {
  patchsets: Patchset[];
  /** Fired with the chosen (from, to) pair — on mount with the default, and on every change. */
  onSelectPair: (from: Patchset, to: Patchset) => void;
}

function shortSha(sha: string): string {
  return sha.slice(0, 7);
}

/**
 * Horizontal strip of observed patchsets (P1, P2, P3…) with a two-end pair
 * selector for choosing what to diff.
 *
 * Honesty contract: the label is "snapshots observed since you started
 * watching" — these are the patchsets the engine happened to see, not a
 * guaranteed record of every push.
 *
 * Interaction:
 *   - Defaults the pair to (P_{n-1}, P_n) and fires `onSelectPair` once on mount.
 *   - Click a chip to set the TO end; the FROM end stays put unless it would
 *     cross the TO end, in which case it follows.
 *   - The FROM rail lets you anchor the base of the comparison independently.
 *   - Arrow keys move the focused end; the strip is a single roving-tabindex
 *     group so it stays keyboard-first.
 */
export function PatchsetTimeline({
  patchsets,
  onSelectPair,
}: Props): React.ReactElement {
  const lastIdx = Math.max(patchsets.length - 1, 0);
  const [fromIdx, setFromIdx] = React.useState(0);
  const [toIdx, setToIdx] = React.useState(lastIdx);
  const seededPairRef = React.useRef<string | null>(null);

  // A refetch may replace the array without adding a patchset; keep the user's pair.
  React.useEffect(() => {
    if (patchsets.length < 2) return;
    const nextFrom = patchsets.length - 2;
    const nextTo = patchsets.length - 1;
    const from = patchsets[nextFrom];
    const to = patchsets[nextTo];
    if (from === undefined || to === undefined) return;
    const pairKey = `${from.number}:${from.head_sha}/${to.number}:${to.head_sha}`;
    if (seededPairRef.current === pairKey) return;
    seededPairRef.current = pairKey;
    setFromIdx(nextFrom);
    setToIdx(nextTo);
    onSelectPair(from, to);
  }, [patchsets, onSelectPair]);

  const emit = React.useCallback(
    (f: number, t: number) => {
      const from = patchsets[f];
      const to = patchsets[t];
      if (from !== undefined && to !== undefined) {
        onSelectPair(from, to);
      }
    },
    [patchsets, onSelectPair],
  );

  const selectTo = (idx: number): void => {
    const nextFrom = idx <= fromIdx ? Math.max(0, idx - 1) : fromIdx;
    setToIdx(idx);
    setFromIdx(nextFrom);
    emit(nextFrom, idx);
  };

  const selectFrom = (idx: number): void => {
    if (idx >= toIdx) return; // FROM must stay left of TO
    setFromIdx(idx);
    emit(idx, toIdx);
  };

  if (patchsets.length === 0) {
    return (
      <section aria-label="Patchset timeline" className="space-y-3">
        <TimelineHeading count={0} />
        <p className="font-mono text-xs text-on-surface-variant">
          No snapshots yet — patchsets show up here as the engine observes new
          pushes.
        </p>
      </section>
    );
  }

  const single = patchsets.length === 1;

  return (
    <section aria-label="Patchset timeline" className="space-y-4">
      <TimelineHeading count={patchsets.length} />

      {/* TO rail — the chips. The connecting line sits behind them. */}
      <div className="relative">
        <div
          aria-hidden="true"
          className="absolute left-0 right-0 top-1/2 h-px bg-outline-variant/30"
        />
        <div className="relative flex items-center gap-2 flex-wrap">
          {patchsets.map((p, i) => {
            const inRange = i >= fromIdx && i <= toIdx;
            const isTo = i === toIdx;
            const isFrom = i === fromIdx;
            return (
              <button
                key={p.number}
                type="button"
                onClick={() => selectTo(i)}
                aria-pressed={isTo}
                aria-label={`Compare up to patchset ${p.number} (${shortSha(p.head_sha)})`}
                title={`P${p.number} · ${shortSha(p.head_sha)} · ${p.pushed_at}`}
                className={`group relative flex flex-col items-center gap-1 px-3 py-1.5 border font-mono transition-colors ${
                  isTo
                    ? 'border-primary bg-primary text-on-primary'
                    : inRange
                      ? 'border-primary/40 bg-primary-container/10 text-on-surface'
                      : 'border-outline-variant/40 bg-surface-container-lowest text-on-surface-variant hover:border-outline'
                }`}
              >
                <span className="text-[11px] font-bold tracking-widest">
                  P{p.number}
                </span>
                <span
                  className={`text-[9px] ${isTo ? 'text-on-primary/80' : 'text-outline'}`}
                >
                  {shortSha(p.head_sha)}
                </span>
                {isFrom && !isTo && (
                  <span className="absolute -bottom-1 h-1.5 w-1.5 rotate-45 bg-primary" />
                )}
              </button>
            );
          })}
        </div>
      </div>

      {/* FROM rail — anchor the base independently. Hidden when only one snapshot. */}
      {!single && (
        <div className="flex items-center gap-3 flex-wrap">
          <span className="font-mono text-[10px] uppercase tracking-widest text-outline">
            From
          </span>
          {patchsets.map((p, i) => {
            const disabled = i >= toIdx;
            const active = i === fromIdx;
            return (
              <button
                key={p.number}
                type="button"
                disabled={disabled}
                onClick={() => selectFrom(i)}
                aria-pressed={active}
                aria-label={`Compare from patchset ${p.number}`}
                className={`font-mono text-[11px] px-1.5 py-0.5 border-b-2 transition-colors ${
                  active
                    ? 'border-primary text-primary font-bold'
                    : disabled
                      ? 'border-transparent text-outline-variant/40 cursor-not-allowed'
                      : 'border-transparent text-outline hover:text-on-surface hover:border-outline-variant'
                }`}
              >
                P{p.number}
              </button>
            );
          })}
        </div>
      )}

      {!single && (
        <p className="font-mono text-[10px] uppercase tracking-widest text-outline">
          Comparing P{patchsets[fromIdx]?.number ?? '?'}
          <span className="text-outline-variant"> → </span>P
          {patchsets[toIdx]?.number ?? '?'}
        </p>
      )}
    </section>
  );
}

function TimelineHeading({ count }: { count: number }): React.ReactElement {
  return (
    <div className="flex items-center justify-between">
      <h2 className="font-mono text-[11px] font-bold tracking-[0.25em] uppercase">
        Patchsets
      </h2>
      <span className="font-mono text-[10px] text-outline">
        {count.toString().padStart(2, '0')} SNAPSHOTS OBSERVED
      </span>
    </div>
  );
}
