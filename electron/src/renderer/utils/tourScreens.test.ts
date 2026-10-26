import { describe, expect, it, vi } from 'vitest';

vi.mock('../router', () => ({
  router: { navigate: vi.fn() },
}));

import { getEffectiveBindings } from './keybindings';
import { resolveTourScreens, TOUR_SCREENS } from './tourScreens';

describe('resolveTourScreens', () => {
  it('resolves 5 screens with formatted key labels from live bindings', () => {
    const resolved = resolveTourScreens(getEffectiveBindings('vim', {}));
    expect(resolved).toHaveLength(TOUR_SCREENS.length);
    expect(resolved.every((s) => s.keys.length > 0)).toBe(true);
    // Spot-check one route-dependent context label renders through.
    const inbox = resolved.find((s) => s.title === 'Inbox & lanes');
    const markLaneRead = inbox?.keys.find((k) => k.label === 'Mark lane read');
    expect(markLaneRead?.context).toBe('in the inbox');
    // Lane jump collapses lane1..lane6 into a single range hint.
    const laneJump = inbox?.keys.find((k) => k.label === 'Jump to a lane');
    expect(laneJump?.keys).toBe('G 1–6');
  });

  it('shows Standard arrow movement on the navigation tour screen', () => {
    const navigation = resolveTourScreens(
      getEffectiveBindings('standard', {}),
    ).find((screen) => screen.title === 'Navigation');
    expect(navigation?.keys.slice(0, 2)).toEqual([
      { keys: '↓', label: 'Move down' },
      { keys: '↑', label: 'Move up' },
    ]);
  });

  it('drops a key hint whose actionId has no matching binding', () => {
    const resolved = resolveTourScreens([]);
    expect(resolved.every((s) => s.keys.length === 0)).toBe(true);
  });
});
