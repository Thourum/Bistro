import { describe, expect, it } from 'vitest';
import type { AppearanceSettings } from '../../types/settings';
import { createTourCompletionPatch } from './tourCompletion';

describe('createTourCompletionPatch', () => {
  it('marks the tour complete without changing other appearance settings', () => {
    const appearance: AppearanceSettings = {
      theme: 'dark',
      uiFont: 'ui-font',
      codeFont: 'code-font',
      uiFontSize: 16,
      codeFontSize: 15,
      highDensity: true,
      onboardingComplete: true,
      tourComplete: false,
    };

    expect(createTourCompletionPatch(appearance)).toEqual({
      appearance: { ...appearance, tourComplete: true },
    });
    expect(appearance.tourComplete).toBe(false);
  });
});
