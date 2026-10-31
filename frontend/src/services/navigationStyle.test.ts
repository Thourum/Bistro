import type { NavigationStyle } from '@renderer/types/settings';
import { describe, expect, it } from 'vitest';
import { neighbourNavigationStyle } from './navigationStyle';

describe('neighbourNavigationStyle', () => {
  it('steps in both directions and wraps at both ends', () => {
    const transitions: readonly (readonly [
      NavigationStyle,
      1 | -1,
      NavigationStyle,
    ])[] = [
      ['vim', 1, 'emacs'],
      ['emacs', 1, 'standard'],
      ['standard', 1, 'vim'],
      ['vim', -1, 'standard'],
      ['standard', -1, 'emacs'],
      ['emacs', -1, 'vim'],
    ];
    for (const [current, delta, next] of transitions) {
      expect(neighbourNavigationStyle(current, delta)).toBe(next);
    }
  });
});
