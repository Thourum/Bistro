import { describe, expect, it, vi } from 'vitest';

vi.mock('../router', () => ({
  router: { navigate: vi.fn() },
}));

import { DEFAULT_KEYBINDINGS, isActive, type KeyBinding } from './keybindings';

function binding(actionId: string): KeyBinding {
  const found = DEFAULT_KEYBINDINGS.find(
    (candidate) => candidate.actionId === actionId,
  );
  if (found === undefined) {
    throw new Error(`Expected binding ${actionId}`);
  }
  return found;
}

describe('inbox keybindings', () => {
  it('defines lane jumps and hides repetitive help entries', () => {
    for (let lane = 1; lane <= 6; lane += 1) {
      const laneBinding = binding(`inbox.lane${lane}`);
      expect(laneBinding.keys).toEqual(['g', String(lane)]);
      expect(laneBinding.hidden).toBe(lane !== 1);
    }
  });

  it('keeps Shift-R route-scoped between inbox and detail actions', () => {
    const inboxMarkRead = binding('inbox.markLaneRead');
    const detailRequestChanges = binding('detail.requestChanges');
    const withoutOverlay = { activeOverlay: null } as const;

    expect(
      isActive(inboxMarkRead, { ...withoutOverlay, routeBase: 'list' }),
    ).toBe(true);
    expect(
      isActive(inboxMarkRead, { ...withoutOverlay, routeBase: 'detail' }),
    ).toBe(false);
    expect(
      isActive(detailRequestChanges, {
        ...withoutOverlay,
        routeBase: 'detail',
      }),
    ).toBe(true);
    expect(
      isActive(detailRequestChanges, { ...withoutOverlay, routeBase: 'list' }),
    ).toBe(false);
  });
});
