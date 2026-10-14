import { describe, expect, it } from 'vitest';
import type { KeyBinding } from './keybindings';
import { formatBindingKeys } from './legendPresets';

function bindingWithKeys(keys: string): KeyBinding {
  return {
    actionId: 'test.action',
    keys,
    description: 'Test',
    activation: {},
  };
}

describe('formatBindingKeys', () => {
  it('renders shifted punctuation as the character it types and keeps other modifiers', () => {
    const plain = { platform: 'mac', useSymbols: false } as const;

    expect(formatBindingKeys(bindingWithKeys('Shift+/'), plain)).toBe('?');
    expect(formatBindingKeys(bindingWithKeys('Shift+;'), plain)).toBe(':');
    expect(formatBindingKeys(bindingWithKeys('Shift+4'), plain)).toBe('$');
    expect(formatBindingKeys(bindingWithKeys('Mod+Shift+/'), plain)).toBe(
      'Cmd+?',
    );
    expect(formatBindingKeys(bindingWithKeys('Shift+G'), plain)).toBe(
      'Shift+G',
    );
  });
});
