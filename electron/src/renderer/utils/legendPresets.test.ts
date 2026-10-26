import { describe, expect, it } from 'vitest';
import { getEffectiveBindings, type KeyBinding } from './keybindings';
import { buildLegendForRouteBase, formatBindingKeys } from './legendPresets';

function bindingWithKeys(keys: string): KeyBinding {
  return {
    actionId: 'test.action',
    keys,
    description: 'Test',
    activation: {},
  };
}

describe('primary movement hints', () => {
  it('shows Standard arrows in the footer and help catalog without duplicate vim aliases', () => {
    const bindings = getEffectiveBindings('standard', {});
    const legend = buildLegendForRouteBase('list', null, bindings);
    expect(legend.slice(0, 2).map((entry) => entry.label)).toEqual(['↓', '↑']);
    expect(
      bindings
        .filter((binding) => binding.actionId.endsWith('Arrow'))
        .every((binding) => binding.hidden !== true),
    ).toBe(true);
    expect(
      getEffectiveBindings('vim', {})
        .filter((binding) => binding.actionId.endsWith('Arrow'))
        .every((binding) => binding.hidden === true),
    ).toBe(true);
  });

  it('prefers movement overrides and falls back only when the primary is absent', () => {
    const bindings = getEffectiveBindings('standard', { 'list.down': ['x'] });
    const legend = buildLegendForRouteBase('list', null, bindings);
    expect(legend[0]?.label).toBe('X');
    expect(
      bindings.find((binding) => binding.actionId === 'list.downArrow')?.hidden,
    ).toBe(true);
    const unbound = getEffectiveBindings('vim', { 'list.down': [] });
    expect(buildLegendForRouteBase('list', null, unbound)[0]?.label).toBe('↓');
  });
});

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
