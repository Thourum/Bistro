import type { HotkeySequence } from '@tanstack/hotkeys';
import { describe, expect, it } from 'vitest';
import { continuesSequence, type KeyPress } from './keybindingActivation';

const TIMEOUT_MS = 1000;

function press(key: string, time: number, ctrlKey = false): KeyPress {
  const event = {
    type: 'keydown',
    key,
    code: /^[a-z]$/i.test(key) ? `Key${key.toUpperCase()}` : `Digit${key}`,
    ctrlKey,
    metaKey: false,
    altKey: false,
    shiftKey: false,
  } as KeyboardEvent;
  return { event, time };
}

const options = {
  sequences: [
    ['G', '1'],
    ['Control+C', 'Control+V'],
  ] satisfies HotkeySequence[],
  timeoutMs: TIMEOUT_MS,
};

describe('continuesSequence', () => {
  it('flags a key that finishes a started sequence', () => {
    expect(continuesSequence([press('g', 0)], press('1', 100), options)).toBe(
      true,
    );
    expect(
      continuesSequence([press('c', 0, true)], press('v', 100, true), options),
    ).toBe(true);
  });

  it('lets the key through on its own or after the sequence timed out', () => {
    expect(continuesSequence([], press('1', 0), options)).toBe(false);
    expect(continuesSequence([press('x', 0)], press('1', 100), options)).toBe(
      false,
    );
    expect(
      continuesSequence([press('g', 0)], press('1', TIMEOUT_MS + 1), options),
    ).toBe(false);
  });
});
