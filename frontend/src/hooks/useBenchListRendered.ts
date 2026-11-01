import { useEffect, useRef } from 'react';

/** Signals the first committed nonempty list without loading the bench bundle. */
export function useBenchListRendered(rows: number): void {
  const hasEmitted = useRef(false);
  useEffect(() => {
    if (
      rows < 1 ||
      hasEmitted.current ||
      !new URLSearchParams(location.search).get('bench_collector')
    ) {
      return;
    }
    hasEmitted.current = true;
    window.dispatchEvent(
      new CustomEvent('bistro:pr-list-rendered', {
        detail: {
          t: performance.now(),
          timeOrigin: performance.timeOrigin,
          rows,
        },
      }),
    );
  }, [rows]);
}
