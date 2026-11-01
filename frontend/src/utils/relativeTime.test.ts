import { expect, it, vi } from 'vitest';
import { relativeTime } from './relativeTime';

it('floors elapsed time and changes units at minute, hour, and day thresholds', () => {
  const now = Date.parse('2026-10-31T12:00:00Z');
  const clock = vi.spyOn(Date, 'now').mockReturnValue(now);
  const cases = [
    [0, '0s ago'],
    [59_999, '59s ago'],
    [60_000, '1m ago'],
    [3_599_999, '59m ago'],
    [3_600_000, '1h ago'],
    [86_399_999, '23h ago'],
    [86_400_000, '1d ago'],
    [172_800_000, '2d ago'],
  ] as const;
  const actual = cases.map(([elapsed]) =>
    relativeTime(new Date(now - elapsed).toISOString()),
  );
  clock.mockRestore();
  expect(actual).toEqual(cases.map(([, expected]) => expected));
});
