// @vitest-environment jsdom
import {
  getHotkeyManager,
  getSequenceManager,
  HotkeyManager,
  type HotkeySequence,
  SequenceManager,
  type SequenceRegistrationView,
} from '@tanstack/hotkeys';
import { afterEach, describe, expect, it } from 'vitest';
import {
  continuesSequence,
  hotkeyOptions,
  isStylePickerTarget,
  yieldsToFocusedControl,
} from './keybindingActivation';

afterEach(() => {
  HotkeyManager.resetInstance();
  SequenceManager.resetInstance();
  document.body.replaceChildren();
});

const TIMEOUT_MS = 1000;
const LAST_KEY_TIME = 100;

function press(
  key: string,
  modifiers: { ctrlKey?: boolean } = {},
): KeyboardEvent {
  return {
    type: 'keydown',
    key,
    code: /^[a-z]$/i.test(key) ? `Key${key.toUpperCase()}` : `Digit${key}`,
    ctrlKey: modifiers.ctrlKey ?? false,
    metaKey: false,
    altKey: false,
    shiftKey: false,
    repeat: false,
    isComposing: false,
  } as KeyboardEvent;
}

function sequence(
  steps: HotkeySequence,
  matchedStepCount: number,
): SequenceRegistrationView {
  return {
    id: steps.join(' '),
    sequence: steps,
    options: { enabled: true, ignoreInputs: true },
    target: {} as Document,
    triggerCount: 0,
    hasFired: false,
    matchedStepCount,
    partialMatchLastKeyTime: LAST_KEY_TIME,
  };
}

const options = {
  sequences: [sequence(['G', '1'], 1), sequence(['Control+C', 'Control+V'], 1)],
  timeoutMs: TIMEOUT_MS,
  now: LAST_KEY_TIME + 1,
};

describe('installed TanStack dispatch', () => {
  it('keeps style cycling inside the picker and never cancels Tab traversal', () => {
    const picker = document.createElement('fieldset');
    picker.dataset.navigationStylePicker = '';
    const card = document.createElement('button');
    picker.append(card);
    const continueButton = document.createElement('button');
    document.body.append(picker, continueButton);
    const cycle = { actionId: 'onboarding.nextStyle', keys: 'Tab' };
    let cycleCount = 0;
    getHotkeyManager().register(
      'Tab',
      (event) => {
        if (!isStylePickerTarget(event.target)) return;
        cycleCount += 1;
      },
      hotkeyOptions(cycle),
    );
    card.focus();
    const pickerTab = new KeyboardEvent('keydown', {
      key: 'Tab',
      bubbles: true,
      cancelable: true,
    });
    card.dispatchEvent(pickerTab);
    expect(cycleCount).toBe(1);
    expect(pickerTab.defaultPrevented).toBe(false);
    continueButton.focus();
    const continueTab = new KeyboardEvent('keydown', {
      key: 'Tab',
      bubbles: true,
      cancelable: true,
    });
    continueButton.dispatchEvent(continueTab);
    expect(cycleCount).toBe(1);
    expect(continueTab.defaultPrevented).toBe(false);
    // Emacs's practice group-toggle Tab must also yield on focused controls.
    expect(yieldsToFocusedControl({ keys: 'Tab' }, card)).toBe(true);
    expect(
      hotkeyOptions({ actionId: 'list.toggleGroup', keys: 'Tab' })
        .preventDefault,
    ).toBe(false);
  });

  it('ignores textarea prefixes, but reports a real sequence before document dispatch', () => {
    const manager = getSequenceManager();
    let filterCount = 0;
    manager.register(
      ['F', 'N'],
      () => {
        filterCount += 1;
      },
      { ignoreInputs: true },
    );
    const textarea = document.createElement('textarea');
    document.body.append(textarea);
    textarea.focus();
    textarea.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'f', bubbles: true }),
    );
    textarea.blur();
    const next = new KeyboardEvent('keydown', { key: 'n', bubbles: true });
    const currentOptions = () => ({
      sequences: [...manager.registrations.state.values()],
      timeoutMs: TIMEOUT_MS,
      now: Date.now(),
    });
    expect(continuesSequence(next, currentOptions())).toBe(false);
    document.dispatchEvent(next);
    expect(filterCount).toBe(0);
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'f', bubbles: true }),
    );
    expect(continuesSequence(next, currentOptions())).toBe(true);
    document.dispatchEvent(next);
    expect(filterCount).toBe(1);
    expect(continuesSequence(next, currentOptions())).toBe(false);
  });

  it('preserves native Enter on a focused button and C-g in overlay inputs', () => {
    const button = document.createElement('button');
    document.body.append(button);
    button.focus();
    const viewDiff = { actionId: 'detail.viewDiff', keys: 'Enter' };
    let diffCount = 0;
    getHotkeyManager().register(
      'Enter',
      (event) => {
        if (yieldsToFocusedControl(viewDiff, event.target)) return;
        event.preventDefault();
        diffCount += 1;
      },
      hotkeyOptions(viewDiff),
    );
    const enter = new KeyboardEvent('keydown', {
      key: 'Enter',
      bubbles: true,
      cancelable: true,
    });
    button.dispatchEvent(enter);
    expect(enter.defaultPrevented).toBe(false);
    expect(diffCount).toBe(0);
    const input = document.createElement('input');
    document.body.append(input);
    input.focus();
    let cancelCount = 0;
    const cancel = { actionId: 'global.cancel', keys: 'Control+G' };
    const handle = getHotkeyManager().register(
      'Control+G',
      () => {
        cancelCount += 1;
      },
      hotkeyOptions(cancel, 'command'),
    );
    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'g', ctrlKey: true, bubbles: true }),
    );
    expect(cancelCount).toBe(1);
    handle.setOptions(hotkeyOptions(cancel, null));
    input.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'g', ctrlKey: true, bubbles: true }),
    );
    expect(cancelCount).toBe(1);
  });

  it('requires release before confirming a held approval key', () => {
    let approvalGestures = 0;
    getHotkeyManager().register(
      'A',
      () => {
        approvalGestures += 1;
      },
      hotkeyOptions({ actionId: 'detail.approve', keys: 'a' }),
    );
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'a', bubbles: true }),
    );
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'a', repeat: true, bubbles: true }),
    );
    expect(approvalGestures).toBe(1);
    document.dispatchEvent(
      new KeyboardEvent('keyup', { key: 'a', bubbles: true }),
    );
    document.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'a', bubbles: true }),
    );
    expect(approvalGestures).toBe(2);
  });
});

describe('continuesSequence', () => {
  it('flags a key that advances actual sequence-manager progress', () => {
    expect(continuesSequence(press('1'), options)).toBe(true);
    expect(continuesSequence(press('v', { ctrlKey: true }), options)).toBe(
      true,
    );
    expect(continuesSequence(press('x'), options)).toBe(false);
  });

  it('does not invent progress from input-only keys or disabled sequences', () => {
    expect(
      continuesSequence(press('n'), {
        ...options,
        sequences: [sequence(['F', 'N'], 0)],
      }),
    ).toBe(false);
    expect(
      continuesSequence(press('1'), {
        ...options,
        sequences: options.sequences.map((registration) => ({
          ...registration,
          options: { enabled: false },
        })),
      }),
    ).toBe(false);
  });

  it('lets timed-out, repeated and composing keys through', () => {
    expect(
      continuesSequence(press('1'), {
        ...options,
        now: LAST_KEY_TIME + TIMEOUT_MS + 1,
      }),
    ).toBe(false);
    expect(continuesSequence({ ...press('1'), repeat: true }, options)).toBe(
      false,
    );
    expect(
      continuesSequence({ ...press('1'), isComposing: true }, options),
    ).toBe(false);
  });
});
