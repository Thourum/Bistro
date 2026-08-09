import { describe, expect, it, vi } from 'vitest';

vi.mock('../router', () => ({
  router: { navigate: vi.fn() },
}));

import { DEFAULT_KEYBINDINGS } from './keybindings';
import { resolveTourScreens, TOUR_SCREENS } from './tourScreens';

describe('resolveTourScreens', () => {
  it('resolves 5 screens with formatted key labels from live bindings', () => {
    const resolved = resolveTourScreens(DEFAULT_KEYBINDINGS);
    expect(resolved).toHaveLength(TOUR_SCREENS.length);
    expect(resolved.every((s) => s.keys.length > 0)).toBe(true);
    // Spot-check one route-dependent context label renders through.
    const inbox = resolved.find((s) => s.title === 'Inbox & lanes');
    const markLaneRead = inbox?.keys.find((k) => k.label === 'Mark lane read');
    expect(markLaneRead?.context).toBe('in the inbox');
  });

  it('drops a key hint whose actionId has no matching binding', () => {
    const resolved = resolveTourScreens([]);
    expect(resolved.every((s) => s.keys.length === 0)).toBe(true);
  });
});
