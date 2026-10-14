// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import type { PR } from '../types';
import { useVisiblePRs } from './useVisiblePRs';

vi.mock('./useWebSocketPRList', () => ({
  useWebSocketPRList: () => ({ data: undefined, isLoading: true, error: null }),
}));

vi.mock('./useStore', () => ({
  useStore: (_store: unknown, selector: (state: unknown) => unknown) =>
    selector(storeState),
}));

vi.mock('../store', () => ({ appStore: {} }));
vi.mock('../store/settings', () => ({ settingsStore: {} }));

const storeState = {
  filterState: 'open',
  sortBy: 'updated',
  searchQuery: '',
  repoFilter: null,
  viewerLogin: '',
  prList: { view: 'grouped' },
};

function Probe({ onPRs }: { onPRs: (prs: PR[]) => void }): null {
  const { prs } = useVisiblePRs();
  onPRs(prs);
  return null;
}

describe('useVisiblePRs', () => {
  let root: Root;
  let container: HTMLDivElement;

  beforeEach(() => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => root.unmount());
    container.remove();
  });

  test('returns the same prs array identity across re-renders while no data has loaded', async () => {
    const seenPRs: PR[][] = [];
    const onPRs = (prs: PR[]): void => {
      seenPRs.push(prs);
    };

    await act(async () => root.render(<Probe onPRs={onPRs} />));
    await act(async () => root.render(<Probe onPRs={onPRs} />));

    expect(seenPRs).toHaveLength(2);
    expect(seenPRs[1]).toBe(seenPRs[0]);
  });
});
