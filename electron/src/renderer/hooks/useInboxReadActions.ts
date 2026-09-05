import { useEffect } from 'react';
import { appStore } from '../store';
import { settingsStore } from '../store/settings';
import type { PR } from '../types';
import { sectionContaining } from '../utils/lanes';
import { useMarkPRsRead } from './useMarkPRsRead';
import { useStore } from './useStore';

const MARK_LANE_READ_EVENT = 'bistro:inbox:mark-lane-read';

/** Wires the list-only Shift-R event to the currently selected inbox lane. */
export function useInboxReadActions(prs: readonly PR[]): void {
  const view = useStore(settingsStore, (state) => state.prList.view);
  const selectedPRIndex = useStore(appStore, (state) => state.selectedPRIndex);
  const laneSections = useStore(
    appStore,
    (state) => state.laneSectionsSnapshot,
  );
  const { markPRsRead } = useMarkPRsRead();

  useEffect(() => {
    const markLaneRead = (): void => {
      if (view === 'grouped' || prs.length === 0) return;
      const section = sectionContaining(laneSections, selectedPRIndex);
      if (section === null) return;

      const prIds = prs
        .slice(section.startIndex, section.startIndex + section.count)
        .map((pr) => pr.id);
      if (prIds.length > 0) {
        void markPRsRead(prIds);
      }
    };

    document.addEventListener(MARK_LANE_READ_EVENT, markLaneRead);
    return () => {
      document.removeEventListener(MARK_LANE_READ_EVENT, markLaneRead);
    };
  }, [laneSections, markPRsRead, prs, selectedPRIndex, view]);
}
